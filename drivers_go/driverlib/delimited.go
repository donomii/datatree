package driverlib

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// ListDelimited exposes headings and string-valued rows for a delimited table.
// Quoted separators, multiline cells and ragged records retain their values.
func ListDelimited(raw []byte, delimiter rune, format string) ([]Resource, map[string][]byte, error) {
	reader := csv.NewReader(bytes.NewReader(raw))
	reader.Comma = delimiter
	// Accept ragged rows; the row objects simply carry the cells present.
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	switch {
	case err != nil:
		return nil, nil, fmt.Errorf("cannot parse the %s document: expected %q-separated records, got %d bytes with error: %v", format, delimiter, len(raw), err)
	case len(records) == 0:
		return nil, map[string][]byte{}, nil
	default:
	}

	header := records[0]
	dataRows := records[1:]
	columns := columnNames(header)

	var resources []Resource
	children := map[string][]byte{}

	resources = AppendKV(resources, "rows", strconv.Itoa(len(dataRows)))
	resources = AppendKV(resources, "columns", strconv.Itoa(len(header)))

	headerJSON, headerErr := json.Marshal(columns)
	switch {
	case headerErr != nil:
		return nil, nil, fmt.Errorf("cannot encode the %s header row as JSON: expected %d column names to marshal, got error: %v", format, len(columns), headerErr)
	default:
		resources = append(resources, Resource{Name: "[headers]", Kind: "json", Size: int64(len(headerJSON))})
		children["[headers]"] = headerJSON
	}

	for index, row := range dataRows {
		name := "[" + strconv.Itoa(index) + "]"
		payload := rowObject(columns, row)
		resources = append(resources, Resource{Name: name, Kind: "json", Size: int64(len(payload))})
		children[name] = payload
	}
	return resources, children, nil
}

// columnNames turns the header record into unique object keys: an empty
// header cell becomes "column N" (1-based), and a duplicate name gets a
// " (N)" suffix until it is unique, so no cell is lost to a key collision.
func columnNames(header []string) []string {
	names := make([]string, 0, len(header))
	seen := map[string]bool{}
	for index, cell := range header {
		name := strings.TrimSpace(cell)
		switch {
		case name == "":
			name = "column " + strconv.Itoa(index+1)
		default:
		}
		candidate := name
		for suffix := 2; seen[candidate]; suffix++ {
			candidate = name + " (" + strconv.Itoa(suffix) + ")"
		}
		seen[candidate] = true
		names = append(names, candidate)
	}
	return names
}

// rowObject renders one data row as a JSON object in column order. Cells
// beyond the header width are keyed "column N" (uniquified against the row's
// keys), and a row shorter than the header simply omits the missing columns.
func rowObject(columns []string, row []string) []byte {
	seen := map[string]bool{}
	for _, name := range columns {
		seen[name] = true
	}
	var out bytes.Buffer
	out.WriteByte('{')
	for index, cell := range row {
		var key string
		switch {
		case index < len(columns):
			key = columns[index]
		default:
			key = "column " + strconv.Itoa(index+1)
			candidate := key
			for suffix := 2; seen[candidate]; suffix++ {
				candidate = key + " (" + strconv.Itoa(suffix) + ")"
			}
			seen[candidate] = true
			key = candidate
		}
		switch {
		case index > 0:
			out.WriteByte(',')
		default:
		}
		out.Write(jsonString(key))
		out.WriteByte(':')
		out.Write(jsonString(cell))
	}
	out.WriteByte('}')
	return out.Bytes()
}

// jsonString marshals one string with full JSON escaping. Marshalling a plain
// string cannot fail, so a failure is a programming error worth a panic (the
// harness reports it as a driver abort).
func jsonString(value string) []byte {
	encoded, err := json.Marshal(value)
	switch {
	case err != nil:
		panic(fmt.Sprintf("cannot encode the table cell %q as a JSON string: %v", value, err))
	default:
		return encoded
	}
}
