// drivers_go/csv_driver/csv_driver.go
package csvdriver

import (
	"bytes"

	"datahumper/drivers_go/driverlib"
)

// csv_driver previews a CSV document as structured data. The first record is
// the header. The view shows "rows" and "columns" counts, a [headers] child
// carrying the header names as a JSON array, and one expandable child per
// data row named [0], [1], and so on. Each row child serves the row as a JSON
// object keyed by column name, so json_driver browses every cell as a
// "column: value" leaf. The driver never touches the network; it only parses
// bytes handed down by its parent.

func Run() {
	driverlib.Run("CSV driver", listCSV)
}

func listCSV(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	switch {
	case len(bytes.TrimSpace(raw)) == 0:
		return nil, map[string][]byte{}, nil
	default:
		return driverlib.ListDelimited(raw, ',', "CSV")
	}
}
