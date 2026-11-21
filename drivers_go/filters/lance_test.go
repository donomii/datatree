package filters

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"datahumper/drivers_go/driverlib"
)

func lanceProtoVarint(number uint64, value uint64) []byte {
	raw := binary.AppendUvarint(nil, number<<3)
	return binary.AppendUvarint(raw, value)
}

func lanceProtoBytes(number uint64, value []byte) []byte {
	raw := binary.AppendUvarint(nil, number<<3|2)
	raw = binary.AppendUvarint(raw, uint64(len(value)))
	return append(raw, value...)
}

func lanceTestFile() []byte {
	field := lanceProtoVarint(1, 2)
	field = append(field, lanceProtoBytes(2, []byte("vector"))...)
	field = append(field, lanceProtoVarint(3, 7)...)
	field = append(field, lanceProtoBytes(5, []byte("fixed_size_list:3"))...)
	field = append(field, lanceProtoVarint(6, 1)...)
	metadataEntry := lanceProtoBytes(1, []byte("ARROW:extension:name"))
	metadataEntry = append(metadataEntry, lanceProtoBytes(2, []byte("lance.vector"))...)
	field = append(field, lanceProtoBytes(10, metadataEntry)...)
	schema := lanceProtoBytes(1, field)
	descriptor := lanceProtoBytes(1, schema)
	descriptor = append(descriptor, lanceProtoVarint(2, 3)...)
	raw := []byte("DATA")
	descriptorPosition := uint64(len(raw))
	raw = append(raw, descriptor...)
	columnPosition := uint64(len(raw))
	raw = append(raw, []byte{8, 1}...)
	columnTablePosition := uint64(len(raw))
	columnEntry := make([]byte, 16)
	binary.LittleEndian.PutUint64(columnEntry[0:8], columnPosition)
	binary.LittleEndian.PutUint64(columnEntry[8:16], 2)
	raw = append(raw, columnEntry...)
	globalTablePosition := uint64(len(raw))
	globalEntry := make([]byte, 16)
	binary.LittleEndian.PutUint64(globalEntry[0:8], descriptorPosition)
	binary.LittleEndian.PutUint64(globalEntry[8:16], uint64(len(descriptor)))
	raw = append(raw, globalEntry...)
	footer := make([]byte, lanceFooterSize)
	binary.LittleEndian.PutUint64(footer[0:8], columnPosition)
	binary.LittleEndian.PutUint64(footer[8:16], columnTablePosition)
	binary.LittleEndian.PutUint64(footer[16:24], globalTablePosition)
	binary.LittleEndian.PutUint32(footer[24:28], 1)
	binary.LittleEndian.PutUint32(footer[28:32], 1)
	binary.LittleEndian.PutUint16(footer[32:34], 2)
	binary.LittleEndian.PutUint16(footer[34:36], 1)
	copy(footer[36:40], "LANC")
	return append(raw, footer...)
}

func TestLanceRowsUseSourceBuiltReader(t *testing.T) {
	workingDirectory, workingErr := os.Getwd()
	switch {
	case workingErr != nil:
		t.Fatal(workingErr)
	default:
	}
	helper := filepath.Join(workingDirectory, "..", "..", "lance_helper", "target", "release", "datatree-lance-helper")
	switch _, statErr := os.Stat(helper); {
	case os.IsNotExist(statErr):
		t.Skip("source-built Lance row reader has not been built")
	case statErr != nil:
		t.Fatal(statErr)
	default:
	}
	t.Setenv("DATATREE_LANCE_HELPER", helper)
	raw, decodeErr := base64.StdEncoding.DecodeString("MABISEhISEhISEhISEhISEhISEhISEhISEhISEhISEhISEhISEhISEhISEhISEhISEhISEhISEhISEhISEhISAAAGAD+/v7+AQAAAAAAAAACAAAAAAAAAAMAAAAAAAAASEhISEhISEhISEhISEhISEhISEhISEhISEhISEhISEhAAEhISEhISEhISEhISEhISEhISEhISEhISEhISEhISEhISEhISEhISEhISEhISEhISEhISEhISEhISEhISEhIAAAcAP7+/v4QAAAAEwAAABYAAAAbAAAAb25ldHdvdGhyZWVI/v7+/khISEhISEhISEhISEhISEhISEhISEhISAo9ChoSAmlkIP///////////wEqBWludDY0MAE4AQofEgRuYW1lGAEg////////////ASoGc3RyaW5nMAE4AhADCikSJwolCh8vbGFuY2UuZW5jb2RpbmdzLkNvbHVtbkVuY29kaW5nEgIKABJACgIAQBICAiAYAyI0EjIKMAodL2xhbmNlLmVuY29kaW5nczIxLlBhZ2VMYXlvdXQSDwoNGgQKAghAMgEBOAFIAwopEicKJQofL2xhbmNlLmVuY29kaW5ncy5Db2x1bW5FbmNvZGluZxICCgASRgoEgAHAARICAigYAyI4EjYKNAodL2xhbmNlLmVuY29kaW5nczIxLlBhZ2VMYXlvdXQSEwoRGggSBgoECgIIIDIBATgBSANBAQAAAAAAAG0AAAAAAAAArgEAAAAAAABzAAAAAAAAAAABAAAAAAAAQQAAAAAAAABBAQAAAAAAACECAAAAAAAAQQIAAAAAAAABAAAAAgAAAAIAAQBMQU5D")
	switch {
	case decodeErr != nil:
		t.Fatal(decodeErr)
	default:
	}
	resources, children, err := driverlib.WithSource(listLance)(raw)
	switch {
	case err != nil:
		t.Fatal(err)
	case string(childBytes(t, resources, children, "[row 0]")) != `{"id":1,"name":"one"}`:
		t.Fatalf("unexpected first Lance row: %s", childBytes(t, resources, children, "[row 0]"))
	case string(childBytes(t, resources, children, "[row 2]")) != `{"id":3,"name":"three"}`:
		t.Fatalf("unexpected final Lance row: %s", childBytes(t, resources, children, "[row 2]"))
	default:
	}
}

func TestLanceFileMetadataAndExactBuffers(t *testing.T) {
	raw := lanceTestFile()
	resources, children, err := driverlib.WithSource(listLance)(raw)
	switch {
	case err != nil:
		t.Fatal(err)
	default:
	}
	schemaBytes := childBytes(t, resources, children, "[schema]")
	var schema lanceSchemaView
	switch err := json.Unmarshal(schemaBytes, &schema); {
	case err != nil:
		t.Fatal(err)
	case schema.Rows != 3 || schema.Version != "2.1" || len(schema.Fields) != 1:
		t.Fatalf("unexpected Lance schema summary: %+v", schema)
	case schema.Fields[0].Name != "vector" || schema.Fields[0].ID != 7 || schema.Fields[0].LogicalType != "fixed_size_list:3" || !schema.Fields[0].Nullable:
		t.Fatalf("unexpected Lance field: %+v", schema.Fields[0])
	case len(schema.Fields[0].Metadata) != 1 || schema.Fields[0].Metadata[0].Key != "ARROW:extension:name" || string(schema.Fields[0].Metadata[0].Value) != "lance.vector":
		t.Fatalf("unexpected Lance field metadata: %+v", schema.Fields[0].Metadata)
	default:
	}
	switch column := childBytes(t, resources, children, "[column 0 metadata]"); {
	case !bytes.Equal(column, []byte{8, 1}):
		t.Fatalf("column metadata changed: %v", column)
	default:
	}
	switch {
	case !bytes.Equal(childBytes(t, resources, children, "[source bytes]"), raw):
		t.Fatal("Lance reader lost exact source bytes")
	case driverlib.DetectKind(raw) != "lance":
		t.Fatalf("Lance footer signature detected as %s", driverlib.DetectKind(raw))
	default:
	}
}

func TestMalformedLanceFilesAreRejected(t *testing.T) {
	fixtures := map[string][]byte{
		"short footer": []byte("LANC"),
		"wrong magic":  append(bytes.Clone(lanceTestFile()[:len(lanceTestFile())-4]), []byte("NOPE")...),
		"old version": func() []byte {
			raw := lanceTestFile()
			binary.LittleEndian.PutUint16(raw[len(raw)-8:len(raw)-6], 0)
			return raw
		}(),
		"bad global table": func() []byte {
			raw := lanceTestFile()
			binary.LittleEndian.PutUint64(raw[len(raw)-24:len(raw)-16], uint64(len(raw)))
			return raw
		}(),
		"bad descriptor": func() []byte {
			raw := lanceTestFile()
			raw[4] = 0
			return raw
		}(),
	}
	for name, raw := range fixtures {
		t.Run(name, func(t *testing.T) {
			resources, children, err := driverlib.WithSource(listLance)(raw)
			switch {
			case err != nil || len(resources) != 2 || resources[0].Kind != "error":
				t.Fatalf("expected visible Lance error and source, got %+v, %v", resources, err)
			case !bytes.Equal(childBytes(t, resources, children, "[source bytes]"), raw):
				t.Fatal("malformed Lance input lost source bytes")
			default:
			}
		})
	}
}
