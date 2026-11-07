// drivers_go/pdf_driver/pdf_driver.go
package pdfdriver

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"

	"datahumper/drivers_go/driverlib"
)

// pdf_driver previews a PDF document without a full PDF parser: it reads the
// version from the %PDF- header, counts the page objects visible in the
// uncompressed body, and lists the link targets declared in /URI actions as
// url leaves. In PDFs that store their objects in compressed object streams
// the page markers are not visible, so the page count is omitted rather than
// reported wrongly. Bytes without a %PDF- header produce a single error
// resource. It is a leaf driver and never touches the network; it only parses
// bytes handed down by its parent.

// headerPattern captures the version digits of the %PDF-1.x header, which the
// format requires near the start of the file (a preamble of at most 1024
// bytes is tolerated by real readers).
var headerPattern = regexp.MustCompile(`%PDF-(\d+\.\d+)`)

// pagePattern matches uncompressed page objects. The \b keeps /Pages (the
// page-tree node) from being counted as a page.
var pagePattern = regexp.MustCompile(`/Type\s*/Page\b`)

// uriPattern captures absolute http(s) targets of /URI link actions.
var uriPattern = regexp.MustCompile(`/URI\s*\(\s*(https?://[^)\s]+)`)

// headerSearchLimit bounds the %PDF- header search to the tolerated preamble.
const headerSearchLimit = 1024

func Run() {
	driverlib.Run("PDF driver", listPDF)
}

func listPDF(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	switch {
	case len(bytes.TrimSpace(raw)) == 0:
		return nil, map[string][]byte{}, nil
	default:
	}

	head := raw
	switch {
	case len(head) > headerSearchLimit:
		head = head[:headerSearchLimit]
	default:
	}
	header := headerPattern.FindSubmatch(head)
	switch {
	case header == nil:
		return nil, nil, fmt.Errorf("cannot preview the PDF: expected a %%PDF- version header within the first %d bytes, got %d bytes without one", headerSearchLimit, len(raw))
	default:
	}

	var resources []driverlib.Resource
	resources = driverlib.AppendKV(resources, "version", string(header[1]))

	pages := len(pagePattern.FindAll(raw, -1))
	switch {
	case pages > 0:
		resources = driverlib.AppendKV(resources, "pages", strconv.Itoa(pages))
	default:
		// Object streams hide the page markers; omit the count instead of
		// reporting zero pages for a real document.
	}

	seen := map[string]bool{}
	for _, match := range uriPattern.FindAllSubmatch(raw, -1) {
		url := string(match[1])
		switch {
		case seen[url]:
			continue
		default:
			seen[url] = true
			resources = append(resources, driverlib.Resource{Name: url, Kind: "url", Size: int64(len(url)), Payload: []byte(url)})
		}
	}
	return resources, map[string][]byte{}, nil
}
