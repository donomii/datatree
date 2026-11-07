// drivers_go/k8s_pod_driver/k8s_pod_driver.go
package k8spoddriver

import (
	"encoding/json"
	"fmt"
	"strconv"

	"datahumper/drivers_go/driverlib"
)

// k8s_pod_driver presents a pod manifest (JSON from kubectl get -o json,
// served by kubernetes_driver) as a curated view: the important scalars as
// "key: value" leaves, one expandable child per container, and expandable
// [labels], [conditions], [volumes], and [manifest] children. The [manifest]
// child always carries the complete manifest, so no data is unreachable. The
// driver never runs kubectl or touches the network; it only parses bytes
// handed down by its parent.

// PodSpec covers the spec scalars this view summarizes.
type PodSpec struct {
	NodeName           string `json:"nodeName"`
	ServiceAccountName string `json:"serviceAccountName"`
}

// PodContainerStatus covers the per-container runtime facts this view shows.
type PodContainerStatus struct {
	Name         string `json:"name"`
	Ready        bool   `json:"ready"`
	RestartCount int64  `json:"restartCount"`
}

// PodStatus covers the status scalars this view summarizes.
type PodStatus struct {
	Phase             string               `json:"phase"`
	PodIP             string               `json:"podIP"`
	StartTime         string               `json:"startTime"`
	ContainerStatuses []PodContainerStatus `json:"containerStatuses"`
}

func Run() {
	driverlib.Run("Pod driver", buildPodView)
}

func buildPodView(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {

	doc := driverlib.RawObject(json.RawMessage(raw))
	switch {
	case len(doc) == 0:
		return nil, nil, fmt.Errorf("cannot parse the pod manifest: expected a JSON object from kubectl get -o json, got %d bytes that do not decode as one", len(raw))
	default:
	}

	var spec PodSpec
	switch err := json.Unmarshal(driverlib.RawField(doc, "spec"), &spec); {
	case err != nil:
		// Optional section; everything stays reachable via [manifest].
	default:
	}
	var status PodStatus
	switch err := json.Unmarshal(driverlib.RawField(doc, "status"), &status); {
	case err != nil:
		// Optional section; everything stays reachable via [manifest].
	default:
	}

	var resources []driverlib.Resource
	children := map[string][]byte{}

	resources = driverlib.AppendKV(resources, "phase", status.Phase)
	resources = driverlib.AppendKV(resources, "node", spec.NodeName)
	resources = driverlib.AppendKV(resources, "podIP", status.PodIP)
	resources = driverlib.AppendKV(resources, "serviceAccount", spec.ServiceAccountName)
	resources = driverlib.AppendKV(resources, "started", status.StartTime)

	restarts := int64(0)
	statusByName := map[string]string{}
	for _, containerStatus := range status.ContainerStatuses {
		restarts += containerStatus.RestartCount
		statusByName[containerStatus.Name] = fmt.Sprintf("ready %t, restarts %d", containerStatus.Ready, containerStatus.RestartCount)
	}
	resources = driverlib.AppendKV(resources, "restarts", strconv.FormatInt(restarts, 10))

	resources, children = driverlib.AppendContainers(resources, children, driverlib.RawAtPath(doc, "spec", "containers"), statusByName)
	resources, children = driverlib.AppendSection(resources, children, "[labels]", driverlib.RawAtPath(doc, "metadata", "labels"))
	resources, children = driverlib.AppendSection(resources, children, "[conditions]", driverlib.RawAtPath(doc, "status", "conditions"))
	resources, children = driverlib.AppendSection(resources, children, "[volumes]", driverlib.RawAtPath(doc, "spec", "volumes"))
	resources, children = driverlib.AppendSection(resources, children, "[manifest]", json.RawMessage(raw))
	return resources, children, nil
}
