// drivers_go/k8s_workload_driver/k8s_workload_driver.go
package k8sworkloaddriver

import (
	"encoding/json"
	"fmt"
	"strconv"

	"datahumper/drivers_go/driverlib"
)

// k8s_workload_driver presents a workload manifest — Deployment, StatefulSet,
// DaemonSet, ReplicaSet, Job, or CronJob — as a curated view: kind, replica
// and schedule facts, rollout strategy, one expandable child per template
// container, and expandable [selector], [conditions], [labels], and
// [manifest] children. The [manifest] child always carries the complete
// manifest, so no data is unreachable. The driver never runs kubectl or
// touches the network; it only parses bytes handed down by its parent.

// WorkloadTop covers the manifest-level scalars this view summarizes.
type WorkloadTop struct {
	Kind string `json:"kind"`
}

// WorkloadStrategy covers the strategy shape shared by Deployments
// (spec.strategy) and StatefulSets/DaemonSets (spec.updateStrategy).
type WorkloadStrategy struct {
	Type string `json:"type"`
}

// WorkloadSpec covers the spec scalars this view summarizes across all
// workload kinds; each kind fills only the fields it has.
type WorkloadSpec struct {
	Replicas       *int64           `json:"replicas"`
	ServiceName    string           `json:"serviceName"`
	Schedule       string           `json:"schedule"`
	Suspend        *bool            `json:"suspend"`
	Strategy       WorkloadStrategy `json:"strategy"`
	UpdateStrategy WorkloadStrategy `json:"updateStrategy"`
}

// WorkloadStatus covers the status counters this view summarizes across all
// workload kinds; each kind fills only the fields it has.
type WorkloadStatus struct {
	Replicas               *int64 `json:"replicas"`
	ReadyReplicas          *int64 `json:"readyReplicas"`
	AvailableReplicas      *int64 `json:"availableReplicas"`
	UpdatedReplicas        *int64 `json:"updatedReplicas"`
	DesiredNumberScheduled *int64 `json:"desiredNumberScheduled"`
	NumberReady            *int64 `json:"numberReady"`
	Active                 *int64 `json:"active"`
	Succeeded              *int64 `json:"succeeded"`
	Failed                 *int64 `json:"failed"`
	LastScheduleTime       string `json:"lastScheduleTime"`
}

func Run() {
	driverlib.Run("Workload driver", buildWorkloadView)
}

func buildWorkloadView(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	switch message, isReadError := driverlib.DecodeDriverReadError(raw); {
	case isReadError:
		return nil, nil, fmt.Errorf("%s", message)
	default:
	}
	doc := driverlib.RawObject(json.RawMessage(raw))
	switch {
	case len(doc) == 0:
		return nil, nil, fmt.Errorf("cannot parse the workload manifest: expected a JSON object from kubectl get -o json, got %d bytes that do not decode as one", len(raw))
	default:
	}

	var top WorkloadTop
	switch err := json.Unmarshal(json.RawMessage(raw), &top); {
	case err != nil:
		// Optional; everything stays reachable via [manifest].
	default:
	}
	var spec WorkloadSpec
	switch err := json.Unmarshal(driverlib.RawField(doc, "spec"), &spec); {
	case err != nil:
		// Optional section; everything stays reachable via [manifest].
	default:
	}
	var status WorkloadStatus
	switch err := json.Unmarshal(driverlib.RawField(doc, "status"), &status); {
	case err != nil:
		// Optional section; everything stays reachable via [manifest].
	default:
	}

	var resources []driverlib.Resource
	children := map[string][]byte{}

	resources = driverlib.AppendKV(resources, "kind", top.Kind)
	resources = driverlib.AppendCount(resources, "desired", spec.Replicas)
	resources = driverlib.AppendCount(resources, "desired", status.DesiredNumberScheduled)
	resources = driverlib.AppendCount(resources, "current", status.Replicas)
	resources = driverlib.AppendCount(resources, "ready", status.ReadyReplicas)
	resources = driverlib.AppendCount(resources, "ready", status.NumberReady)
	resources = driverlib.AppendCount(resources, "available", status.AvailableReplicas)
	resources = driverlib.AppendCount(resources, "updated", status.UpdatedReplicas)
	resources = driverlib.AppendCount(resources, "active", status.Active)
	resources = driverlib.AppendCount(resources, "succeeded", status.Succeeded)
	resources = driverlib.AppendCount(resources, "failed", status.Failed)

	strategy := spec.Strategy.Type
	switch {
	case strategy == "":
		strategy = spec.UpdateStrategy.Type
	default:
	}
	resources = driverlib.AppendKV(resources, "strategy", strategy)
	resources = driverlib.AppendKV(resources, "serviceName", spec.ServiceName)
	resources = driverlib.AppendKV(resources, "schedule", spec.Schedule)
	switch {
	case spec.Suspend != nil:
		resources = driverlib.AppendKV(resources, "suspend", strconv.FormatBool(*spec.Suspend))
	default:
	}
	resources = driverlib.AppendKV(resources, "lastSchedule", status.LastScheduleTime)

	containersRaw := driverlib.RawAtPath(doc, "spec", "template", "spec", "containers")
	switch {
	case len(driverlib.TrimmedBytes(containersRaw)) == 0:
		// CronJobs nest the pod template one level deeper.
		containersRaw = driverlib.RawAtPath(doc, "spec", "jobTemplate", "spec", "template", "spec", "containers")
	default:
	}
	resources, children = driverlib.AppendContainers(resources, children, containersRaw, map[string]string{})

	resources, children = driverlib.AppendSection(resources, children, "[selector]", driverlib.RawAtPath(doc, "spec", "selector"))
	resources, children = driverlib.AppendSection(resources, children, "[conditions]", driverlib.RawAtPath(doc, "status", "conditions"))
	resources, children = driverlib.AppendSection(resources, children, "[labels]", driverlib.RawAtPath(doc, "metadata", "labels"))
	resources, children = driverlib.AppendSection(resources, children, "[manifest]", json.RawMessage(raw))
	return resources, children, nil
}
