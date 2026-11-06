// drivers_go/html_driver/html_driver.go
package htmldriver

import (
	"bytes"
	"html"
	"regexp"
	"strings"

	"datahumper/drivers_go/driverlib"
)

// html_driver previews an HTML document: the page title becomes a "title"
// leaf, and every absolute http(s) URL referenced by an href or src attribute
// becomes a url leaf, in document order with duplicates removed. It is a leaf
// driver (nothing is servable) and never touches the network; it only parses
// bytes handed down by its parent.

// titlePattern captures the first <title> element's contents, case-insensitive
// and across newlines.
var titlePattern = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

// linkPattern captures absolute http(s) URLs in href and src attributes. The
// character class stops at quotes, whitespace, and '>' so unquoted attribute
// values also terminate correctly.
var linkPattern = regexp.MustCompile(`(?i)(?:href|src)\s*=\s*["']?(https?://[^"'\s>]+)`)

func Run() {
	driverlib.Run("HTML driver", listHTML)
}

func listHTML(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	switch {
	case len(bytes.TrimSpace(raw)) == 0:
		return nil, map[string][]byte{}, nil
	default:
	}

	var resources []driverlib.Resource
	resources = driverlib.AppendKV(resources, "title", pageTitle(raw))

	seen := map[string]bool{}
	for _, match := range linkPattern.FindAllSubmatch(raw, -1) {
		// Entity-decode so href="a?x=1&amp;y=2" lists the real URL.
		url := html.UnescapeString(string(match[1]))
		switch {
		case seen[url]:
			continue
		default:
			seen[url] = true
			resources = append(resources, driverlib.Resource{Name: url, Kind: "url", Size: int64(len(url))})
		}
	}
	return resources, map[string][]byte{}, nil
}

// pageTitle extracts the first <title> element, entity-decoded and with runs
// of whitespace collapsed to single spaces. A missing or empty title yields
// "" so the caller's AppendKV skips the row.
func pageTitle(raw []byte) string {
	match := titlePattern.FindSubmatch(raw)
	switch {
	case match == nil:
		return ""
	default:
		return strings.Join(strings.Fields(html.UnescapeString(string(match[1]))), " ")
	}
}
