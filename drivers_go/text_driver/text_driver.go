// drivers_go/text_driver/text_driver.go
package textdriver

import (
	"regexp"

	"datahumper/drivers_go/driverlib"
)

// text_driver scans text for http(s) URLs and lists each match as a url leaf.
// It is a leaf driver: nothing is servable, so it exits after emitting.

var urlPattern = regexp.MustCompile(`https?://[^\s'">]+`)

func Run() {
	driverlib.Run("Text driver", listURLs)
}

func listURLs(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	var resources []driverlib.Resource
	for _, url := range urlPattern.FindAllString(string(raw), -1) {
		resources = append(resources, driverlib.Resource{Name: url, Kind: "url", Size: int64(len(url)), Payload: []byte(url)})
	}
	return resources, map[string][]byte{}, nil
}
