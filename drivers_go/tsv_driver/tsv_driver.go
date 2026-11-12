package tsvdriver

import "datahumper/drivers_go/driverlib"

// Run exposes tab-separated headings and rows through the ordinary byte protocol.
func Run() {
	driverlib.Run("TSV driver", listTSV)
}

func listTSV(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	return driverlib.ListDelimited(raw, '\t', "TSV")
}
