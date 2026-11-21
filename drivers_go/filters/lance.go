package filters

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"datahumper/drivers_go/driverlib"
)

const lanceFooterSize = 40

type lanceFooter struct {
	columnMetadataStart uint64
	columnTableStart    uint64
	globalTableStart    uint64
	globalBufferCount   uint32
	columnCount         uint32
	majorVersion        uint16
	minorVersion        uint16
}

type lanceBuffer struct {
	position uint64
	size     uint64
}

type lanceMetadataView struct {
	Key   string `json:"key"`
	Value []byte `json:"value"`
}

type lanceFieldView struct {
	Name        string              `json:"name"`
	ID          int32               `json:"id"`
	ParentID    int32               `json:"parent_id"`
	Type        string              `json:"type"`
	LogicalType string              `json:"logical_type"`
	Nullable    bool                `json:"nullable"`
	Metadata    []lanceMetadataView `json:"metadata,omitempty"`
}

type lanceSchemaView struct {
	Rows     uint64              `json:"rows"`
	Version  string              `json:"version"`
	Fields   []lanceFieldView    `json:"fields"`
	Metadata []lanceMetadataView `json:"metadata,omitempty"`
}

type lanceWireValue struct {
	number  uint64
	wire    uint64
	integer uint64
	bytes   []byte
}

func listLance(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	footer, footerErr := parseLanceFooter(raw)
	switch {
	case footerErr != nil:
		return nil, nil, footerErr
	default:
	}
	columnBuffers, columnErr := parseLanceBufferTable(raw, footer.columnTableStart, footer.columnCount, "column metadata")
	switch {
	case columnErr != nil:
		return nil, nil, columnErr
	default:
	}
	globalBuffers, globalErr := parseLanceBufferTable(raw, footer.globalTableStart, footer.globalBufferCount, "global buffer")
	switch {
	case globalErr != nil:
		return nil, nil, globalErr
	case len(globalBuffers) == 0:
		return nil, nil, fmt.Errorf("cannot read Lance file: expected global buffer 0 to contain the FileDescriptor, got no global buffers")
	default:
	}
	descriptorBytes := raw[globalBuffers[0].position : globalBuffers[0].position+globalBuffers[0].size]
	rows, fields, metadata, descriptorErr := parseLanceDescriptor(descriptorBytes)
	switch {
	case descriptorErr != nil:
		return nil, nil, descriptorErr
	default:
	}
	version := fmt.Sprintf("%d.%d", footer.majorVersion, footer.minorVersion)
	schemaBytes, marshalErr := json.Marshal(lanceSchemaView{Rows: rows, Version: version, Fields: fields, Metadata: metadata})
	switch {
	case marshalErr != nil:
		panic(fmt.Sprintf("cannot encode validated Lance schema with %d fields: %v", len(fields), marshalErr))
	default:
	}
	resources := []driverlib.Resource{
		driverlib.KV("format version", version),
		driverlib.KV("rows", strconv.FormatUint(rows, 10)),
		driverlib.KV("columns", strconv.FormatUint(uint64(footer.columnCount), 10)),
		driverlib.KV("schema fields", strconv.Itoa(len(fields))),
		driverlib.KV("global buffers", strconv.FormatUint(uint64(footer.globalBufferCount), 10)),
	}
	children := map[string][]byte{}
	resources = appendChild(resources, children, "[schema]", "json", schemaBytes)
	for index, buffer := range columnBuffers {
		name := fmt.Sprintf("[column %d metadata]", index)
		resources = appendChild(resources, children, name, "protobuf", raw[buffer.position:buffer.position+buffer.size])
	}
	for index, buffer := range globalBuffers {
		name := fmt.Sprintf("[global buffer %d]", index)
		switch index {
		case 0:
			name = "[file descriptor protobuf]"
		default:
		}
		resources = appendChild(resources, children, name, "protobuf", raw[buffer.position:buffer.position+buffer.size])
	}
	resources, children = appendLanceRows(resources, children, raw)
	return resources, children, nil
}

func appendLanceRows(resources []driverlib.Resource, children map[string][]byte, raw []byte) ([]driverlib.Resource, map[string][]byte) {
	helper, helperErr := lanceHelperPath()
	switch {
	case helperErr != nil:
		return append(resources, driverlib.Resource{Name: helperErr.Error(), Kind: "error"}), children
	default:
	}
	temporary, createErr := os.CreateTemp("", "datatree-lance-*.lance")
	switch {
	case createErr != nil:
		return append(resources, driverlib.Resource{Name: fmt.Sprintf("cannot prepare Lance rows: %v", createErr), Kind: "error"}), children
	default:
	}
	temporaryName := temporary.Name()
	temporaryClosed := false
	defer func() {
		var closeErr error
		switch {
		case temporaryClosed:
		default:
			closeErr = temporary.Close()
		}
		removeErr := os.Remove(temporaryName)
		switch {
		case closeErr != nil:
			panic(fmt.Sprintf("cannot close temporary Lance file %s: %v", temporaryName, closeErr))
		case removeErr != nil:
			panic(fmt.Sprintf("cannot remove temporary Lance file %s: %v", temporaryName, removeErr))
		default:
		}
	}()
	switch _, writeErr := temporary.Write(raw); {
	case writeErr != nil:
		return append(resources, driverlib.Resource{Name: fmt.Sprintf("cannot prepare Lance rows in %s: %v", temporaryName, writeErr), Kind: "error"}), children
	default:
	}
	switch closeErr := temporary.Close(); {
	case closeErr != nil:
		return append(resources, driverlib.Resource{Name: fmt.Sprintf("cannot finish temporary Lance file %s: %v", temporaryName, closeErr), Kind: "error"}), children
	default:
		temporaryClosed = true
	}
	command := exec.Command(helper, temporaryName)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	stdout, pipeErr := command.StdoutPipe()
	switch {
	case pipeErr != nil:
		return append(resources, driverlib.Resource{Name: fmt.Sprintf("cannot read Lance row output: %v", pipeErr), Kind: "error"}), children
	default:
	}
	switch startErr := command.Start(); {
	case startErr != nil:
		return append(resources, driverlib.Resource{Name: fmt.Sprintf("cannot start Lance row reader %s: %v", helper, startErr), Kind: "error"}), children
	default:
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 64*1024*1024)
	row := 0
	var rowErr error
	for scanner.Scan() {
		rowBytes := bytes.Clone(scanner.Bytes())
		switch {
		case !json.Valid(rowBytes):
			if rowErr == nil {
				rowErr = fmt.Errorf("cannot decode Lance row %d: helper returned invalid JSON", row)
			} else {
			}
		default:
			resources = appendChild(resources, children, fmt.Sprintf("[row %d]", row), "json", rowBytes)
		}
		row++
	}
	waitErr := command.Wait()
	switch {
	case scanner.Err() != nil:
		return append(resources, driverlib.Resource{Name: fmt.Sprintf("cannot read Lance rows: %v", scanner.Err()), Kind: "error"}), children
	case rowErr != nil:
		return append(resources, driverlib.Resource{Name: rowErr.Error(), Kind: "error"}), children
	case waitErr != nil:
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = waitErr.Error()
		} else {
		}
		return append(resources, driverlib.Resource{Name: message, Kind: "error"}), children
	default:
		return resources, children
	}
}

func lanceHelperPath() (string, error) {
	configured := strings.TrimSpace(os.Getenv("DATATREE_LANCE_HELPER"))
	switch {
	case configured != "":
		return configured, nil
	default:
	}
	executable, executableErr := os.Executable()
	switch {
	case executableErr != nil:
		return "", fmt.Errorf("cannot locate Lance row reader: DATATREE_LANCE_HELPER is unset and executable path lookup failed: %v", executableErr)
	default:
	}
	directory := filepath.Dir(executable)
	candidates := []string{
		filepath.Join(directory, "datatree-lance-helper"),
		filepath.Join(directory, "lance_helper", "target", "release", "datatree-lance-helper"),
	}
	for _, candidate := range candidates {
		info, statErr := os.Stat(candidate)
		switch {
		case statErr == nil && !info.IsDir():
			return candidate, nil
		case statErr == nil:
		case os.IsNotExist(statErr):
		default:
			return "", fmt.Errorf("cannot inspect Lance row reader %s: %v", candidate, statErr)
		}
	}
	return "", fmt.Errorf("Lance row values require datatree-lance-helper beside Datatree; rebuild with build.xsh")
}

func parseLanceFooter(raw []byte) (lanceFooter, error) {
	switch {
	case len(raw) < lanceFooterSize:
		return lanceFooter{}, fmt.Errorf("cannot read Lance file: expected at least %d bytes for the footer, got %d", lanceFooterSize, len(raw))
	case string(raw[len(raw)-4:]) != "LANC":
		return lanceFooter{}, fmt.Errorf("cannot read Lance file: expected final magic LANC, got %q", raw[len(raw)-4:])
	default:
	}
	footerBytes := raw[len(raw)-lanceFooterSize:]
	footer := lanceFooter{
		columnMetadataStart: binary.LittleEndian.Uint64(footerBytes[0:8]),
		columnTableStart:    binary.LittleEndian.Uint64(footerBytes[8:16]),
		globalTableStart:    binary.LittleEndian.Uint64(footerBytes[16:24]),
		globalBufferCount:   binary.LittleEndian.Uint32(footerBytes[24:28]),
		columnCount:         binary.LittleEndian.Uint32(footerBytes[28:32]),
		majorVersion:        binary.LittleEndian.Uint16(footerBytes[32:34]),
		minorVersion:        binary.LittleEndian.Uint16(footerBytes[34:36]),
	}
	footerStart := uint64(len(raw) - lanceFooterSize)
	switch {
	case footer.majorVersion < 2:
		return lanceFooter{}, fmt.Errorf("cannot read Lance file version %d.%d: this reader supports the version 2 footer and FileDescriptor layout", footer.majorVersion, footer.minorVersion)
	case footer.columnMetadataStart > footer.columnTableStart:
		return lanceFooter{}, fmt.Errorf("cannot read Lance footer: column metadata start %d is after column table start %d", footer.columnMetadataStart, footer.columnTableStart)
	case footer.columnTableStart > footer.globalTableStart:
		return lanceFooter{}, fmt.Errorf("cannot read Lance footer: column table start %d is after global table start %d", footer.columnTableStart, footer.globalTableStart)
	case footer.globalTableStart > footerStart:
		return lanceFooter{}, fmt.Errorf("cannot read Lance footer: global table start %d is after footer start %d", footer.globalTableStart, footerStart)
	default:
		return footer, nil
	}
}

func parseLanceBufferTable(raw []byte, start uint64, count uint32, description string) ([]lanceBuffer, error) {
	tableSize := uint64(count) * 16
	footerStart := uint64(len(raw) - lanceFooterSize)
	switch {
	case start > footerStart || tableSize > footerStart-start:
		return nil, fmt.Errorf("cannot read Lance %s table: %d entries at byte %d exceed footer start %d", description, count, start, footerStart)
	default:
	}
	buffers := make([]lanceBuffer, 0, count)
	for index := uint32(0); index < count; index++ {
		offset := start + uint64(index)*16
		buffer := lanceBuffer{position: binary.LittleEndian.Uint64(raw[offset : offset+8]), size: binary.LittleEndian.Uint64(raw[offset+8 : offset+16])}
		switch {
		case buffer.position > uint64(len(raw)) || buffer.size > uint64(len(raw))-buffer.position:
			return nil, fmt.Errorf("cannot read Lance %s %d: byte range %d..%d exceeds file size %d", description, index, buffer.position, buffer.position+buffer.size, len(raw))
		default:
			buffers = append(buffers, buffer)
		}
	}
	return buffers, nil
}

func parseLanceDescriptor(raw []byte) (uint64, []lanceFieldView, []lanceMetadataView, error) {
	var rows uint64
	var schema []byte
	for offset := 0; offset < len(raw); {
		value, next, err := nextLanceWireValue(raw, offset)
		switch {
		case err != nil:
			return 0, nil, nil, fmt.Errorf("cannot decode Lance FileDescriptor at byte %d: %v", offset, err)
		default:
			offset = next
		}
		switch value.number {
		case 1:
			switch {
			case value.wire != 2 || schema != nil:
				return 0, nil, nil, fmt.Errorf("cannot decode Lance FileDescriptor schema: expected one length-delimited field 1")
			default:
				schema = value.bytes
			}
		case 2:
			switch value.wire {
			case 0:
				rows = value.integer
			default:
				return 0, nil, nil, fmt.Errorf("cannot decode Lance FileDescriptor rows: expected varint field 2, got wire type %d", value.wire)
			}
		default:
		}
	}
	switch {
	case schema == nil:
		return 0, nil, nil, fmt.Errorf("cannot decode Lance FileDescriptor: required schema field 1 is absent")
	default:
	}
	fields, metadata, err := parseLanceSchema(schema)
	switch {
	case err != nil:
		return 0, nil, nil, err
	default:
		return rows, fields, metadata, nil
	}
}

func parseLanceSchema(raw []byte) ([]lanceFieldView, []lanceMetadataView, error) {
	fields := []lanceFieldView{}
	metadata := []lanceMetadataView{}
	for offset := 0; offset < len(raw); {
		value, next, err := nextLanceWireValue(raw, offset)
		switch {
		case err != nil:
			return nil, nil, fmt.Errorf("cannot decode Lance schema at byte %d: %v", offset, err)
		default:
			offset = next
		}
		switch value.number {
		case 1:
			field, fieldErr := parseLanceField(value)
			switch {
			case fieldErr != nil:
				return nil, nil, fieldErr
			default:
				fields = append(fields, field)
			}
		case 5:
			entry, entryErr := parseLanceMetadata(value, "schema")
			switch {
			case entryErr != nil:
				return nil, nil, entryErr
			default:
				metadata = append(metadata, entry)
			}
		default:
		}
	}
	return fields, metadata, nil
}

func parseLanceField(value lanceWireValue) (lanceFieldView, error) {
	switch value.wire {
	case 2:
	default:
		return lanceFieldView{}, fmt.Errorf("cannot decode Lance schema field: expected length-delimited field 1, got wire type %d", value.wire)
	}
	field := lanceFieldView{}
	fieldType := uint64(0)
	for offset := 0; offset < len(value.bytes); {
		item, next, err := nextLanceWireValue(value.bytes, offset)
		switch {
		case err != nil:
			return lanceFieldView{}, fmt.Errorf("cannot decode Lance field at byte %d: %v", offset, err)
		default:
			offset = next
		}
		switch item.number {
		case 1:
			if item.wire != 0 {
				return lanceFieldView{}, fmt.Errorf("cannot decode Lance field type: expected varint wire type 0, got %d", item.wire)
			} else {
			}
			fieldType = item.integer
		case 2:
			if item.wire != 2 {
				return lanceFieldView{}, fmt.Errorf("cannot decode Lance field name: expected length-delimited wire type 2, got %d", item.wire)
			} else {
			}
			field.Name = string(item.bytes)
		case 3:
			if item.wire != 0 {
				return lanceFieldView{}, fmt.Errorf("cannot decode Lance field id: expected varint wire type 0, got %d", item.wire)
			} else {
			}
			field.ID = int32(item.integer)
		case 4:
			if item.wire != 0 {
				return lanceFieldView{}, fmt.Errorf("cannot decode Lance field parent id: expected varint wire type 0, got %d", item.wire)
			} else {
			}
			field.ParentID = int32(item.integer)
		case 5:
			if item.wire != 2 {
				return lanceFieldView{}, fmt.Errorf("cannot decode Lance field logical type: expected length-delimited wire type 2, got %d", item.wire)
			} else {
			}
			field.LogicalType = string(item.bytes)
		case 6:
			if item.wire != 0 || item.integer > 1 {
				return lanceFieldView{}, fmt.Errorf("cannot decode Lance field nullable flag: expected Boolean varint 0 or 1, got wire type %d value %d", item.wire, item.integer)
			} else {
			}
			field.Nullable = item.integer != 0
		case 10:
			entry, entryErr := parseLanceMetadata(item, "field")
			switch {
			case entryErr != nil:
				return lanceFieldView{}, entryErr
			default:
				field.Metadata = append(field.Metadata, entry)
			}
		default:
		}
	}
	switch fieldType {
	case 0:
		field.Type = "parent"
	case 1:
		field.Type = "repeated"
	case 2:
		field.Type = "leaf"
	default:
		return lanceFieldView{}, fmt.Errorf("cannot decode Lance field %q: unknown field type %d", field.Name, fieldType)
	}
	switch {
	case !utf8.ValidString(field.Name) || !utf8.ValidString(field.LogicalType):
		return lanceFieldView{}, fmt.Errorf("cannot decode Lance field %d: name or logical type is not valid UTF-8", field.ID)
	default:
		return field, nil
	}
}

func parseLanceMetadata(value lanceWireValue, description string) (lanceMetadataView, error) {
	switch value.wire {
	case 2:
	default:
		return lanceMetadataView{}, fmt.Errorf("cannot decode Lance %s metadata: expected length-delimited map entry, got wire type %d", description, value.wire)
	}
	entry := lanceMetadataView{}
	keySeen := false
	for offset := 0; offset < len(value.bytes); {
		item, next, err := nextLanceWireValue(value.bytes, offset)
		switch {
		case err != nil:
			return lanceMetadataView{}, fmt.Errorf("cannot decode Lance %s metadata at byte %d: %v", description, offset, err)
		default:
			offset = next
		}
		switch item.number {
		case 1:
			if item.wire != 2 {
				return lanceMetadataView{}, fmt.Errorf("cannot decode Lance %s metadata key: expected length-delimited wire type 2, got %d", description, item.wire)
			} else {
			}
			entry.Key, keySeen = string(item.bytes), true
		case 2:
			if item.wire != 2 {
				return lanceMetadataView{}, fmt.Errorf("cannot decode Lance %s metadata value: expected length-delimited wire type 2, got %d", description, item.wire)
			} else {
			}
			entry.Value = item.bytes
		default:
		}
	}
	switch {
	case !keySeen || !utf8.ValidString(entry.Key):
		return lanceMetadataView{}, fmt.Errorf("cannot decode Lance %s metadata: expected a valid UTF-8 key", description)
	default:
		return entry, nil
	}
}

func nextLanceWireValue(raw []byte, offset int) (lanceWireValue, int, error) {
	switch {
	case offset < 0 || offset >= len(raw):
		return lanceWireValue{}, offset, fmt.Errorf("expected a Protocol Buffers tag within %d bytes", len(raw))
	default:
	}
	tag, tagSize := binary.Uvarint(raw[offset:])
	switch {
	case tagSize <= 0 || tag>>3 == 0:
		return lanceWireValue{}, offset, fmt.Errorf("invalid Protocol Buffers tag")
	default:
		offset += tagSize
	}
	value := lanceWireValue{number: tag >> 3, wire: tag & 7}
	switch value.wire {
	case 0:
		integer, size := binary.Uvarint(raw[offset:])
		switch {
		case size <= 0:
			return lanceWireValue{}, offset, fmt.Errorf("truncated varint field %d", value.number)
		default:
			value.integer, offset = integer, offset+size
		}
	case 1:
		switch {
		case len(raw)-offset < 8:
			return lanceWireValue{}, offset, fmt.Errorf("truncated fixed64 field %d", value.number)
		default:
			value.bytes, offset = raw[offset:offset+8], offset+8
		}
	case 2:
		length, size := binary.Uvarint(raw[offset:])
		switch {
		case size <= 0 || length > uint64(len(raw)-offset-size):
			return lanceWireValue{}, offset, fmt.Errorf("invalid length-delimited field %d", value.number)
		default:
			offset += size
			value.bytes, offset = raw[offset:offset+int(length)], offset+int(length)
		}
	case 5:
		switch {
		case len(raw)-offset < 4:
			return lanceWireValue{}, offset, fmt.Errorf("truncated fixed32 field %d", value.number)
		default:
			value.bytes, offset = raw[offset:offset+4], offset+4
		}
	default:
		return lanceWireValue{}, offset, fmt.Errorf("unsupported Protocol Buffers wire type %d for field %d", value.wire, value.number)
	}
	return value, offset, nil
}
