package filters

import (
	"bytes"
	"encoding/asn1"
	"encoding/binary"
	"fmt"
	"strconv"
	"unicode/utf8"

	"datahumper/drivers_go/driverlib"
)

func listASN1(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	var values []asn1.RawValue
	remaining := raw
	for len(remaining) != 0 {
		var value asn1.RawValue
		next, err := asn1.Unmarshal(remaining, &value)
		switch {
		case err != nil:
			return nil, nil, fmt.Errorf("cannot parse ASN.1 DER at byte %d: %v", len(raw)-len(remaining), err)
		default:
			values = append(values, value)
			remaining = next
		}
	}
	switch {
	case len(values) == 0:
		return nil, nil, fmt.Errorf("cannot parse ASN.1: expected at least one DER value")
	case len(values) == 1 && values[0].IsCompound:
		return listASN1Contents(values[0])
	case len(values) == 1:
		value := values[0]
		kind := "raw"
		switch {
		case value.Class == 0 && (value.Tag == 12 || value.Tag == 19 || value.Tag == 22 || value.Tag == 23 || value.Tag == 24) && utf8.Valid(value.Bytes):
			kind = "text"
		default:
		}
		return single(fmt.Sprintf("[class %d tag %d value]", value.Class, value.Tag), kind, value.Bytes)
	default:
		var resources []driverlib.Resource
		children := map[string][]byte{}
		for index, value := range values {
			resources = appendChild(resources, children, fmt.Sprintf("[%d] class %d tag %d", index, value.Class, value.Tag), "asn1", value.FullBytes)
		}
		return resources, children, nil
	}
}

func listASN1Contents(parent asn1.RawValue) ([]driverlib.Resource, map[string][]byte, error) {
	var resources []driverlib.Resource
	children := map[string][]byte{}
	remaining := parent.Bytes
	for len(remaining) != 0 {
		var value asn1.RawValue
		next, err := asn1.Unmarshal(remaining, &value)
		switch {
		case err != nil:
			return nil, nil, fmt.Errorf("cannot parse constructed DER class %d tag %d: %v", parent.Class, parent.Tag, err)
		default:
			resources = appendChild(resources, children, fmt.Sprintf("[%d] class %d tag %d", len(resources), value.Class, value.Tag), "asn1", value.FullBytes)
			remaining = next
		}
	}
	return resources, children, nil
}

func listProtobuf(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	var resources []driverlib.Resource
	children := map[string][]byte{}
	for offset := 0; offset < len(raw); {
		tag, count := binary.Uvarint(raw[offset:])
		switch {
		case count <= 0 || tag>>3 == 0 || tag>>3 > (1<<29)-1:
			return nil, nil, fmt.Errorf("cannot parse Protocol Buffers tag at byte %d", offset)
		default:
			offset += count
		}
		start := offset
		name := fmt.Sprintf("field %d (wire %d)", tag>>3, tag&7)
		kind := "raw"
		switch tag & 7 {
		case 0:
			value, size := binary.Uvarint(raw[offset:])
			switch {
			case size <= 0:
				return nil, nil, fmt.Errorf("cannot read %s varint at byte %d", name, offset)
			default:
				name += " = " + strconv.FormatUint(value, 10)
				offset += size
			}
		case 1:
			offset += 8
		case 5:
			offset += 4
		case 2:
			length, size := binary.Uvarint(raw[offset:])
			switch {
			case size <= 0 || length > uint64(len(raw)-offset-size):
				return nil, nil, fmt.Errorf("cannot read %s length-delimited payload at byte %d", name, offset)
			default:
				offset += size
				start = offset
				offset += int(length)
				kind = driverlib.DetectKind(raw[start:offset])
			}
		default:
			return nil, nil, fmt.Errorf("cannot inspect %s: unsupported wire type; deprecated groups are not handled by this reader", name)
		}
		switch {
		case offset > len(raw):
			return nil, nil, fmt.Errorf("cannot read %s: truncated fixed-width payload", name)
		default:
			resources = appendChild(resources, children, name, kind, raw[start:offset])
		}
	}
	return resources, children, nil
}

func listRIFF(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	switch {
	case len(raw) < 12 || (string(raw[:4]) != "RIFF" && string(raw[:4]) != "RIFX" && string(raw[:4]) != "LIST"):
		return nil, nil, fmt.Errorf("cannot read RIFF: expected a RIFF or LIST header with a form type")
	default:
	}
	var order binary.ByteOrder = binary.LittleEndian
	switch {
	case string(raw[:4]) == "RIFX" || string(raw[:4]) == "LIST" && uint64(order.Uint32(raw[4:8]))+8 != uint64(len(raw)) && uint64(binary.BigEndian.Uint32(raw[4:8]))+8 == uint64(len(raw)):
		order = binary.BigEndian
	default:
	}
	switch {
	case uint64(order.Uint32(raw[4:8]))+8 != uint64(len(raw)):
		return nil, nil, fmt.Errorf("cannot read RIFF: declared size %d does not match %d input bytes", uint64(order.Uint32(raw[4:8]))+8, len(raw))
	default:
	}
	resources := []driverlib.Resource{driverlib.KV("form type", string(raw[8:12]))}
	children := map[string][]byte{}
	for offset := 12; offset < len(raw); {
		switch {
		case len(raw)-offset < 8:
			return nil, nil, fmt.Errorf("cannot read RIFF chunk header at byte %d", offset)
		default:
		}
		name := string(raw[offset : offset+4])
		length := uint64(order.Uint32(raw[offset+4 : offset+8]))
		end := uint64(offset) + 8 + length
		padded := end + length%2
		switch {
		case padded > uint64(len(raw)):
			return nil, nil, fmt.Errorf("cannot read RIFF chunk %q at byte %d: length %d exceeds input", name, offset, length)
		default:
		}
		data := raw[offset+8 : int(end)]
		kind := driverlib.DetectKind(data)
		switch name {
		case "LIST", "RIFF":
			kind, data = "riff", raw[offset:int(end)]
		default:
		}
		resources = appendChild(resources, children, name, kind, data)
		offset = int(padded)
	}
	return resources, children, nil
}

func listMP4(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	var resources []driverlib.Resource
	children := map[string][]byte{}
	for offset := 0; offset < len(raw); {
		switch {
		case len(raw)-offset < 8:
			return nil, nil, fmt.Errorf("cannot read ISO media box header at byte %d", offset)
		default:
		}
		length := uint64(binary.BigEndian.Uint32(raw[offset : offset+4]))
		name := string(raw[offset+4 : offset+8])
		header := 8
		switch length {
		case 0:
			length = uint64(len(raw) - offset)
		case 1:
			switch {
			case len(raw)-offset < 16:
				return nil, nil, fmt.Errorf("cannot read extended ISO media box %q", name)
			default:
				header, length = 16, binary.BigEndian.Uint64(raw[offset+8:offset+16])
			}
		default:
		}
		switch {
		case length < uint64(header) || length > uint64(len(raw)-offset):
			return nil, nil, fmt.Errorf("cannot read ISO media box %q at byte %d: invalid length %d", name, offset, length)
		default:
		}
		data := raw[offset+header : offset+int(length)]
		kind := "raw"
		switch name {
		case "moov", "trak", "mdia", "minf", "stbl", "edts", "dinf", "udta", "moof", "traf", "mvex", "mfra", "iprp", "ipco":
			kind = "mp4"
		case "meta":
			switch {
			case len(data) < 4:
				return nil, nil, fmt.Errorf("cannot read meta full box: missing version and flags")
			default:
				// Keep version/flags readable separately from nested boxes.
				resources = appendChild(resources, children, "meta version/flags", "raw", data[:4])
				data, kind = data[4:], "mp4"
			}
		default:
			// Unknown boxes keep their complete payload without guessing a schema.
		}
		resources = appendChild(resources, children, name, kind, bytes.Clone(data))
		offset += int(length)
	}
	return resources, children, nil
}
