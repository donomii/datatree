// drivers_go/json_driver/json_driver.go
package jsondriver

import (
	"datahumper/drivers_go/driverlib"
)

// json_driver browses a JSON document as hierarchical data. Objects and
// arrays become expandable children that mount another json_driver, so a
// kubernetes manifest (or any JSON file) can be drilled to any depth. Scalar
// leaves display their value inline. It never touches the network or kubectl:
// it only parses bytes handed down by its parent driver.
//
// To avoid the escape type interface{}, every nested value is kept as a
// json.RawMessage (a byte slice). Objects decode into map[string]RawMessage
// and arrays into []RawMessage, so the driver classifies and serves each
// value from its raw bytes without ever materialising a dynamic Go value.
// The whole walk lives in driverlib.ListGenericJSON, which the kubernetes
// presenter drivers reuse for their [manifest] children.

func Run() {
	driverlib.Run("JSON driver", driverlib.ListGenericJSON)
}
