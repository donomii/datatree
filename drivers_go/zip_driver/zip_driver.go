// drivers_go/zip_driver/zip_driver.go
package zipdriver

import (
	"archive/zip"
	"bytes"
	"io"

	"datahumper/drivers_go/driverlib"
)

// zip_driver parses the upstream bytes as a ZIP archive and lists one
// resource per file entry, classified by extension so nested archives stay
// expandable. Entry bytes are kept in memory and served as child ranges.
// Bytes that are not a ZIP archive produce an empty listing, matching the
// driver's original silent-skip behavior.

func Run() {
	driverlib.Run("ZIP driver", listZipEntries)
}

func listZipEntries(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	switch {
	case err != nil:
		// not a zip; return empty
		return nil, map[string][]byte{}, nil
	default:
	}
	resList := []driverlib.Resource{}
	entryMap := map[string][]byte{}
	for _, f := range zr.File {
		switch {
		case f.FileInfo().IsDir():
			// ZIP directory placeholders carry no bytes and would confuse
			// directory_driver, which expects real filesystem paths.
			continue
		default:
		}
		r, openErr := f.Open()
		switch {
		case openErr != nil:
			continue
		default:
		}
		buf := new(bytes.Buffer)
		_, copyErr := io.Copy(buf, r)
		closeErr := r.Close()
		switch {
		case copyErr != nil || closeErr != nil:
			continue
		default:
		}
		data := buf.Bytes()
		entryMap[f.Name] = data
		resList = append(resList, driverlib.Resource{Name: f.Name, Kind: driverlib.KindForName(f.Name), Size: int64(len(data))})
	}
	return resList, entryMap, nil
}
