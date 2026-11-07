// drivers_go/k8s_service_driver/k8s_service_driver.go
package k8sservicedriver

import (
	"encoding/json"
	"fmt"
	"strconv"

	"datahumper/drivers_go/driverlib"
)

// k8s_service_driver presents a Service manifest as a curated view: the
// service type and addresses, one "port ..." leaf per declared port, one
// "selector ..." leaf per selector pair, and expandable [ports], [labels],
// and [manifest] children. The [manifest] child always carries the complete
// manifest, so no data is unreachable. The driver never runs kubectl or
// touches the network; it only parses bytes handed down by its parent.

// ServicePort covers one spec.ports entry. TargetPort stays raw because the
// API allows either an integer or a string there.
type ServicePort struct {
	Name       string          `json:"name"`
	Port       int64           `json:"port"`
	Protocol   string          `json:"protocol"`
	TargetPort json.RawMessage `json:"targetPort"`
	NodePort   int64           `json:"nodePort"`
}

// ServiceSpec covers the spec fields this view summarizes.
type ServiceSpec struct {
	Type        string            `json:"type"`
	ClusterIP   string            `json:"clusterIP"`
	ExternalIPs []string          `json:"externalIPs"`
	Selector    map[string]string `json:"selector"`
	Ports       []ServicePort     `json:"ports"`
}

// LoadBalancerIngress covers one status.loadBalancer.ingress entry.
type LoadBalancerIngress struct {
	IP       string `json:"ip"`
	Hostname string `json:"hostname"`
}

func Run() {
	driverlib.Run("Service driver", buildServiceView)
}

func buildServiceView(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {

	doc := driverlib.RawObject(json.RawMessage(raw))
	switch {
	case len(doc) == 0:
		return nil, nil, fmt.Errorf("cannot parse the service manifest: expected a JSON object from kubectl get -o json, got %d bytes that do not decode as one", len(raw))
	default:
	}

	var spec ServiceSpec
	switch err := json.Unmarshal(driverlib.RawField(doc, "spec"), &spec); {
	case err != nil:
		// Optional section; everything stays reachable via [manifest].
	default:
	}
	var ingresses []LoadBalancerIngress
	switch err := json.Unmarshal(driverlib.RawAtPath(doc, "status", "loadBalancer", "ingress"), &ingresses); {
	case err != nil:
		// Optional section; everything stays reachable via [manifest].
	default:
	}

	var resources []driverlib.Resource
	children := map[string][]byte{}

	serviceType := spec.Type
	switch {
	case serviceType == "":
		serviceType = "ClusterIP"
	default:
	}
	resources = driverlib.AppendKV(resources, "type", serviceType)
	resources = driverlib.AppendKV(resources, "clusterIP", spec.ClusterIP)
	for _, externalIP := range spec.ExternalIPs {
		resources = driverlib.AppendKV(resources, "externalIP", externalIP)
	}
	for _, ingress := range ingresses {
		address := ingress.IP
		switch {
		case address == "":
			address = ingress.Hostname
		default:
		}
		resources = driverlib.AppendKV(resources, "loadBalancer", address)
	}
	for _, port := range spec.Ports {
		resources = driverlib.AppendKV(resources, "port "+portLabel(port), portValue(port))
	}
	for _, key := range driverlib.SortedStringKeys(spec.Selector) {
		resources = driverlib.AppendKV(resources, "selector "+key, spec.Selector[key])
	}

	resources, children = driverlib.AppendSection(resources, children, "[ports]", driverlib.RawAtPath(doc, "spec", "ports"))
	resources, children = driverlib.AppendSection(resources, children, "[labels]", driverlib.RawAtPath(doc, "metadata", "labels"))
	resources, children = driverlib.AppendSection(resources, children, "[manifest]", json.RawMessage(raw))
	return resources, children, nil
}

// portLabel names a port row by the port's name, falling back to its number.
func portLabel(port ServicePort) string {
	switch {
	case port.Name != "":
		return port.Name
	default:
		return strconv.FormatInt(port.Port, 10)
	}
}

// portValue renders "80/TCP -> 8080" plus the node port when present. The
// target falls back to the port itself, matching the kubernetes default.
func portValue(port ServicePort) string {
	protocol := port.Protocol
	switch {
	case protocol == "":
		protocol = "TCP"
	default:
	}
	target := driverlib.RenderScalar(port.TargetPort)
	switch {
	case target == "":
		target = strconv.FormatInt(port.Port, 10)
	default:
	}
	value := strconv.FormatInt(port.Port, 10) + "/" + protocol + " -> " + target
	switch {
	case port.NodePort != 0:
		return value + " (nodePort " + strconv.FormatInt(port.NodePort, 10) + ")"
	default:
		return value
	}
}
