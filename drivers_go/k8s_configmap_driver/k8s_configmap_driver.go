// drivers_go/k8s_configmap_driver/k8s_configmap_driver.go
package k8sconfigmapdriver

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"unicode/utf8"

	"datahumper/drivers_go/driverlib"
)

// k8s_configmap_driver presents a ConfigMap or Secret manifest as its
// payload: every data key becomes a child whose bytes are the entry's value,
// so a mounted config file or certificate is read directly instead of being
// squinted at inside JSON. ConfigMap data values are used as-is, ConfigMap
// binaryData and Secret data values are base64-decoded first, and each entry
// is classified as text or file by UTF-8 validity. A [manifest] child always
// carries the complete manifest. The driver never runs kubectl or touches
// the network; it only parses bytes handed down by its parent. Reading a
// Secret's values is still a read: whether this driver may see them at all
// is decided by the user's kubectl credentials.

// ConfigManifest covers the ConfigMap and Secret fields this view uses.
type ConfigManifest struct {
	Kind       string            `json:"kind"`
	Type       string            `json:"type"`
	Immutable  *bool             `json:"immutable"`
	Data       map[string]string `json:"data"`
	BinaryData map[string]string `json:"binaryData"`
}

func Run() {
	driverlib.Run("ConfigMap driver", buildConfigView)
}

func buildConfigView(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	switch message, isReadError := driverlib.DecodeDriverReadError(raw); {
	case isReadError:
		return nil, nil, fmt.Errorf("%s", message)
	default:
	}
	var manifest ConfigManifest
	switch err := json.Unmarshal(raw, &manifest); {
	case err != nil:
		return nil, nil, fmt.Errorf("cannot parse the configmap or secret manifest: expected a JSON object from kubectl get -o json, got %d bytes with error: %v", len(raw), err)
	default:
	}

	var resources []driverlib.Resource
	children := map[string][]byte{}

	resources = driverlib.AppendKV(resources, "kind", manifest.Kind)
	resources = driverlib.AppendKV(resources, "type", manifest.Type)
	switch {
	case manifest.Immutable != nil:
		resources = driverlib.AppendKV(resources, "immutable", strconv.FormatBool(*manifest.Immutable))
	default:
	}

	// Secrets carry base64 in data; ConfigMaps carry plain strings there.
	valuesAreBase64 := manifest.Kind == "Secret"
	for _, key := range driverlib.SortedStringKeys(manifest.Data) {
		resources, children = appendEntry(resources, children, key, manifest.Data[key], valuesAreBase64)
	}
	for _, key := range driverlib.SortedStringKeys(manifest.BinaryData) {
		resources, children = appendEntry(resources, children, key, manifest.BinaryData[key], true)
	}

	resources, children = driverlib.AppendSection(resources, children, "[manifest]", json.RawMessage(raw))
	return resources, children, nil
}

// appendEntry adds one data entry as a servable child. Entries that fail to
// base64-decode are reported inline as error leaves rather than dropped.
// Entry keys can never collide with the [manifest] child because the API
// restricts them to alphanumerics, '-', '_' and '.'.
func appendEntry(resources []driverlib.Resource, children map[string][]byte, key string, value string, isBase64 bool) ([]driverlib.Resource, map[string][]byte) {
	payload := []byte(value)
	switch {
	case isBase64:
		decoded, err := base64.StdEncoding.DecodeString(value)
		switch {
		case err != nil:
			return append(resources, driverlib.Resource{Name: fmt.Sprintf("%s: expected base64 data, got decode error: %v", key, err), Kind: "error", Size: 0}), children
		default:
			payload = decoded
		}
	default:
	}
	kind := "file"
	switch {
	case utf8.Valid(payload):
		kind = "text"
	default:
	}
	resources = append(resources, driverlib.Resource{Name: key, Kind: kind, Size: int64(len(payload))})
	children[key] = payload
	return resources, children
}
