// drivers_go/k8s_node_driver/k8s_node_driver.go
package k8snodedriver

import (
	"encoding/json"
	"fmt"

	"datahumper/drivers_go/driverlib"
)

// k8s_node_driver presents a Node manifest as a curated view: kubelet and OS
// facts, addresses, capacity and allocatable resources, conditions, and
// taints as "key: value" leaves, plus expandable [labels] and [manifest]
// children. The [manifest] child always carries the complete manifest, so no
// data is unreachable. The driver never runs kubectl or touches the network;
// it only parses bytes handed down by its parent.

// NodeInfo covers the status.nodeInfo scalars this view shows.
type NodeInfo struct {
	KubeletVersion          string `json:"kubeletVersion"`
	OSImage                 string `json:"osImage"`
	OperatingSystem         string `json:"operatingSystem"`
	Architecture            string `json:"architecture"`
	ContainerRuntimeVersion string `json:"containerRuntimeVersion"`
	KernelVersion           string `json:"kernelVersion"`
}

// NodeAddress covers one status.addresses entry.
type NodeAddress struct {
	Type    string `json:"type"`
	Address string `json:"address"`
}

// NodeCondition covers one status.conditions entry.
type NodeCondition struct {
	Type   string `json:"type"`
	Status string `json:"status"`
}

// NodeStatus covers the status fields this view summarizes.
type NodeStatus struct {
	NodeInfo    NodeInfo          `json:"nodeInfo"`
	Addresses   []NodeAddress     `json:"addresses"`
	Capacity    map[string]string `json:"capacity"`
	Allocatable map[string]string `json:"allocatable"`
	Conditions  []NodeCondition   `json:"conditions"`
}

// NodeTaint covers one spec.taints entry.
type NodeTaint struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Effect string `json:"effect"`
}

// NodeSpec covers the spec fields this view summarizes.
type NodeSpec struct {
	PodCIDR       string      `json:"podCIDR"`
	Unschedulable bool        `json:"unschedulable"`
	Taints        []NodeTaint `json:"taints"`
}

func Run() {
	driverlib.Run("Node driver", buildNodeView)
}

func buildNodeView(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	switch message, isReadError := driverlib.DecodeDriverReadError(raw); {
	case isReadError:
		return nil, nil, fmt.Errorf("%s", message)
	default:
	}
	doc := driverlib.RawObject(json.RawMessage(raw))
	switch {
	case len(doc) == 0:
		return nil, nil, fmt.Errorf("cannot parse the node manifest: expected a JSON object from kubectl get -o json, got %d bytes that do not decode as one", len(raw))
	default:
	}

	var spec NodeSpec
	switch err := json.Unmarshal(driverlib.RawField(doc, "spec"), &spec); {
	case err != nil:
		// Optional section; everything stays reachable via [manifest].
	default:
	}
	var status NodeStatus
	switch err := json.Unmarshal(driverlib.RawField(doc, "status"), &status); {
	case err != nil:
		// Optional section; everything stays reachable via [manifest].
	default:
	}

	var resources []driverlib.Resource
	children := map[string][]byte{}

	resources = driverlib.AppendKV(resources, "kubeletVersion", status.NodeInfo.KubeletVersion)
	resources = driverlib.AppendKV(resources, "osImage", status.NodeInfo.OSImage)
	resources = driverlib.AppendKV(resources, "operatingSystem", status.NodeInfo.OperatingSystem)
	resources = driverlib.AppendKV(resources, "architecture", status.NodeInfo.Architecture)
	resources = driverlib.AppendKV(resources, "containerRuntime", status.NodeInfo.ContainerRuntimeVersion)
	resources = driverlib.AppendKV(resources, "kernel", status.NodeInfo.KernelVersion)
	resources = driverlib.AppendKV(resources, "podCIDR", spec.PodCIDR)
	switch {
	case spec.Unschedulable:
		resources = driverlib.AppendKV(resources, "unschedulable", "true")
	default:
	}
	for _, address := range status.Addresses {
		resources = driverlib.AppendKV(resources, "address "+address.Type, address.Address)
	}
	for _, key := range driverlib.SortedStringKeys(status.Capacity) {
		resources = driverlib.AppendKV(resources, "capacity "+key, status.Capacity[key])
	}
	for _, key := range driverlib.SortedStringKeys(status.Allocatable) {
		resources = driverlib.AppendKV(resources, "allocatable "+key, status.Allocatable[key])
	}
	for _, condition := range status.Conditions {
		resources = driverlib.AppendKV(resources, "condition "+condition.Type, condition.Status)
	}
	for _, taint := range spec.Taints {
		label := taint.Key
		switch {
		case taint.Value != "":
			label = label + "=" + taint.Value
		default:
		}
		resources = driverlib.AppendKV(resources, "taint "+label, taint.Effect)
	}

	resources, children = driverlib.AppendSection(resources, children, "[labels]", driverlib.RawAtPath(doc, "metadata", "labels"))
	resources, children = driverlib.AppendSection(resources, children, "[manifest]", json.RawMessage(raw))
	return resources, children, nil
}
