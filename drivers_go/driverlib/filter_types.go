package driverlib

// FilterInfo describes a byte filter in the host API and both browsers.
type FilterInfo struct {
	Driver      string   `json:"driver"`
	Kind        string   `json:"kind"`
	Description string   `json:"description"`
	Extensions  []string `json:"extensions"`
	Text        bool     `json:"text"`
}

// ByteBundle keeps generated table/row containers lossless for binary cells.
type ByteBundle struct {
	Format  string      `json:"format"`
	Entries []ByteEntry `json:"entries"`
}

type ByteEntry struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	Data []byte `json:"data"`
}
