package driverlib

import (
	"bytes"
	"strings"
	"testing"
)

func TestDelimitedTables(t *testing.T) {
	fixtures := []struct {
		name   string
		raw    string
		header string
		row    string
	}{
		{"quoted cells", "name\tnote\r\nAlice, Bob\t\"tab\there\nnext \"\"line\"\"\"\r\n", `["name","note"]`, `{"name":"Alice, Bob","note":"tab\there\nnext \"line\""}`},
		{"empty cells", "\t\n\t\n", `["column 1","column 2"]`, `{"column 1":"","column 2":""}`},
		{"duplicate headers and extra cells", "a\ta\tcolumn 4\n1\t2\t3\t4\n", `["a","a (2)","column 4"]`, `{"a":"1","a (2)":"2","column 4":"3","column 4 (2)":"4"}`},
		{"short row", "a\tb\n1\n", `["a","b"]`, `{"a":"1"}`},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			for _, delimiter := range []rune{'\t', ','} {
				raw := fixture.raw
				switch delimiter {
				case ',':
					// The quoted-cell case deliberately contains a literal comma.
					raw = strings.ReplaceAll(strings.ReplaceAll(raw, "Alice, Bob", "Alice Bob"), "\t", ",")
				default:
				}
				resources, children, err := ListDelimited([]byte(raw), delimiter, "test table")
				wantRow := fixture.row
				switch delimiter {
				case ',':
					wantRow = strings.ReplaceAll(strings.ReplaceAll(wantRow, "Alice, Bob", "Alice Bob"), `\t`, ",")
				default:
				}
				switch {
				case err != nil || len(resources) != 4:
					t.Fatalf("expected counts, headers and row: %+v %v", resources, err)
				case string(children["[headers]"]) != fixture.header || string(children["[0]"]) != wantRow:
					t.Fatalf("delimiter %q lost cell data: headers %s, row %s", delimiter, children["[headers]"], children["[0]"])
				default:
				}
			}
		})
	}
}

func TestTSVEmptyAndRejectedInput(t *testing.T) {
	for _, raw := range []string{"", "\n\r\n"} {
		resources, children, err := ListDelimited([]byte(raw), '\t', "TSV")
		switch {
		case err != nil || len(resources) != 0 || len(children) != 0:
			t.Fatalf("empty TSV returned data: %+v %v", resources, err)
		default:
		}
	}
	input := []byte("name\tnote\nAlice\t\"unterminated")
	view := WithSource(func(raw []byte) ([]Resource, map[string][]byte, error) {
		return ListDelimited(raw, '\t', "TSV")
	})
	resources, children, err := view(input)
	switch {
	case err != nil || len(resources) != 2 || resources[0].Kind != "error" || !strings.Contains(resources[0].Name, "TSV"):
		t.Fatalf("expected descriptive TSV error and source: %+v %v", resources, err)
	case !bytes.Equal(children[resources[1].Key], input):
		t.Fatal("rejected TSV lost original bytes")
	default:
	}
	for _, filename := range []string{"data.tsv", "DATA.TSV"} {
		switch kind := KindForName(filename); kind {
		case "tsv":
			t.Logf("%s classified as TSV", filename)
		default:
			t.Fatalf("%s classified as %s, expected tsv", filename, kind)
		}
	}
}
