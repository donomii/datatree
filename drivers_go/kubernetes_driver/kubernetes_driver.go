// drivers_go/kubernetes_driver/kubernetes_driver.go
package kubernetesdriver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"datahumper/drivers_go/driverlib"
)

// MountState identifies one location in the kubernetes hierarchy. It is the
// byte payload a parent driver serves for an expandable kubernetes child, and
// the payload this driver requests from its parent during initialization.
// root_driver serves "{}" for the virtual kubernetes entry, which means the
// root level (the list of kubeconfig contexts).
type MountState struct {
	Context   string `json:"context,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Cluster   bool   `json:"cluster,omitempty"`
	Type      string `json:"type,omitempty"`
}

// childEntry remembers how to serve one listed child: containers hand their
// mount state to the next driver layer, objects are fetched from kubectl.
type childEntry struct {
	isObject bool
	state    []byte
}

type kubernetesConfig struct {
	kubeconfig     string
	maxResults     int
	maxOutputBytes int64
}

type boundedBuffer struct {
	data  bytes.Buffer
	limit int64
	total int64
}

// clusterScopedEntryName contains characters that are never legal in a
// kubernetes namespace name, so it cannot collide with a real namespace.
const clusterScopedEntryName = "[cluster-scoped]"

const defaultKubernetesMaxResults = 0
const defaultKubernetesMaxOutputBytes int64 = 8 * 1024 * 1024
const maxKubernetesMountStateBytes = 16 * 1024
const maxKubernetesSelectionBytes = 4096

// readOnlyKubectlCommands is the complete set of kubectl subcommands this
// driver may execute. Every one of them only reads kubeconfig or cluster
// state. The driver refuses to run anything else, so it can never modify a
// cluster.
var readOnlyKubectlCommands = map[string]bool{
	"get":           true,
	"api-resources": true,
	"config":        true,
}

func Run() {
	// A blocked kubectl invocation panics before any process is spawned, so
	// the read-only guarantee fails closed at this recovery point.
	defer driverlib.RecoverPanic("Kubernetes driver")

	enc := json.NewEncoder(os.Stdout)
	dec := json.NewDecoder(os.Stdin)

	stateBytes, fetchErr := driverlib.FetchUpstreamBytes(enc, dec)
	switch {
	case fetchErr != nil:
		logKubernetesStartFailure(fetchErr)
		return
	case len(stateBytes) > maxKubernetesMountStateBytes:
		driverlib.EmitError(enc, fmt.Sprintf("cannot parse the kubernetes mount state: received %d bytes, but mount states are limited to %d bytes", len(stateBytes), maxKubernetesMountStateBytes))
		return
	default:
	}

	config, configErr := resolveKubernetesConfig()
	switch {
	case configErr != nil:
		driverlib.EmitError(enc, configErr.Error())
		return
	default:
	}

	state, stateErr := parseMountState(stateBytes)
	switch {
	case stateErr != nil:
		driverlib.EmitError(enc, stateErr.Error())
		return
	default:
	}

	resources, children, listErr := listLevel(config, state)
	switch {
	case listErr != nil:
		driverlib.EmitError(enc, listErr.Error())
		return
	default:
	}

	switch err := enc.Encode(driverlib.ResourcesMsg{Resources: resources}); {
	case err != nil:
		logKubernetesResourceEncodeFailure(state, err)
		return
	default:
	}

	serveChildren(enc, dec, config, state, children)
}

// parseMountState turns the parent-provided bytes into a MountState. Empty
// bytes mean the kubernetes root, which lists kubeconfig contexts.
func parseMountState(stateBytes []byte) (MountState, error) {
	trimmed := strings.TrimSpace(string(stateBytes))
	switch {
	case trimmed == "":
		return MountState{}, nil
	default:
	}
	var state MountState
	switch err := json.Unmarshal([]byte(trimmed), &state); {
	case err != nil:
		return MountState{}, fmt.Errorf("cannot parse the kubernetes mount state: expected a JSON object with optional context, namespace, cluster and type fields, got %q with error: %v", trimmed, err)
	default:
	}
	switch {
	case state.Namespace != "" && state.Cluster:
		return MountState{}, fmt.Errorf("invalid kubernetes mount state %q: namespace and cluster scope are mutually exclusive", trimmed)
	case (state.Namespace != "" || state.Cluster || state.Type != "") && state.Context == "":
		return MountState{}, fmt.Errorf("invalid kubernetes mount state %q: namespace, cluster and type all require a context", trimmed)
	case state.Type != "" && state.Namespace == "" && !state.Cluster:
		return MountState{}, fmt.Errorf("invalid kubernetes mount state %q: a resource type requires either a namespace or cluster scope", trimmed)
	default:
	}
	switch err := validateSelection(state.Context, "context"); {
	case err != nil:
		return MountState{}, err
	default:
	}
	switch err := validateSelection(state.Namespace, "namespace"); {
	case err != nil:
		return MountState{}, err
	default:
	}
	switch err := validateSelection(state.Type, "type"); {
	case err != nil:
		return MountState{}, err
	default:
	}
	return state, nil
}

// listLevel produces the resources for the hierarchy level the state points
// at: contexts, then namespaces plus the cluster-scoped entry, then resource
// types, then the objects of one type.
func listLevel(config kubernetesConfig, state MountState) ([]driverlib.Resource, map[string]childEntry, error) {
	switch {
	case state.Context == "":
		return listContexts(config)
	case state.Type != "":
		return listObjects(config, state)
	case state.Namespace != "":
		return listResourceTypes(config, state, true)
	case state.Cluster:
		return listResourceTypes(config, state, false)
	default:
		return listContextChildren(config, state)
	}
}

func listContexts(config kubernetesConfig) ([]driverlib.Resource, map[string]childEntry, error) {
	out, err := runReadOnlyKubectl(config, []string{"config", "get-contexts", "-o", "name"})
	switch {
	case err != nil:
		return nil, nil, err
	default:
	}
	var resources []driverlib.Resource
	children := map[string]childEntry{}
	lines, total := outputLines(out, config.maxResults)
	for _, contextName := range lines {
		childState, stateErr := encodeMountState(MountState{Context: contextName})
		switch {
		case stateErr != nil:
			return nil, nil, stateErr
		default:
		}
		resources = append(resources, driverlib.Resource{Name: contextName, Kind: "kubernetes", Size: 0})
		children[contextName] = childEntry{state: childState}
	}
	resources = appendTruncationNotice(resources, "kubeconfig contexts", total, config.maxResults)
	return resources, children, nil
}

func listContextChildren(config kubernetesConfig, state MountState) ([]driverlib.Resource, map[string]childEntry, error) {
	out, err := runReadOnlyKubectl(config, []string{"get", "--context=" + state.Context, "namespaces", "-o", "name"})
	switch {
	case err != nil:
		return nil, nil, err
	default:
	}
	var resources []driverlib.Resource
	children := map[string]childEntry{}
	clusterState, clusterErr := encodeMountState(MountState{Context: state.Context, Cluster: true})
	switch {
	case clusterErr != nil:
		return nil, nil, clusterErr
	default:
	}
	resources = append(resources, driverlib.Resource{Name: clusterScopedEntryName, Kind: "kubernetes", Size: 0})
	children[clusterScopedEntryName] = childEntry{state: clusterState}
	lines, total := outputLines(out, config.maxResults)
	for _, line := range lines {
		namespaceName := nameAfterSlash(line)
		childState, stateErr := encodeMountState(MountState{Context: state.Context, Namespace: namespaceName})
		switch {
		case stateErr != nil:
			return nil, nil, stateErr
		default:
		}
		resources = append(resources, driverlib.Resource{Name: namespaceName, Kind: "kubernetes", Size: 0})
		children[namespaceName] = childEntry{state: childState}
	}
	resources = appendTruncationNotice(resources, "namespaces", total, config.maxResults)
	return resources, children, nil
}

func listResourceTypes(config kubernetesConfig, state MountState, namespaced bool) ([]driverlib.Resource, map[string]childEntry, error) {
	namespacedFlag := "--namespaced=false"
	switch {
	case namespaced:
		namespacedFlag = "--namespaced=true"
	default:
	}
	out, err := runReadOnlyKubectl(config, []string{"api-resources", "--context=" + state.Context, "--verbs=list", namespacedFlag, "-o", "name"})
	switch {
	case err != nil:
		return nil, nil, err
	default:
	}
	var resources []driverlib.Resource
	children := map[string]childEntry{}
	lines, total := outputLines(out, config.maxResults)
	for _, typeName := range lines {
		childState, stateErr := encodeMountState(MountState{Context: state.Context, Namespace: state.Namespace, Cluster: state.Cluster, Type: typeName})
		switch {
		case stateErr != nil:
			return nil, nil, stateErr
		default:
		}
		resources = append(resources, driverlib.Resource{Name: typeName, Kind: "kubernetes", Size: 0})
		children[typeName] = childEntry{state: childState}
	}
	resources = appendTruncationNotice(resources, "resource types", total, config.maxResults)
	return resources, children, nil
}

func listObjects(config kubernetesConfig, state MountState) ([]driverlib.Resource, map[string]childEntry, error) {
	switch err := safePositional(state.Type, "kubernetes resource type"); {
	case err != nil:
		return nil, nil, err
	default:
	}
	args := []string{"get", "--context=" + state.Context}
	switch {
	case state.Namespace != "":
		args = append(args, "--namespace="+state.Namespace)
	default:
	}
	args = append(args, state.Type, "-o", "name")
	out, err := runReadOnlyKubectl(config, args)
	switch {
	case err != nil:
		return nil, nil, err
	default:
	}
	var resources []driverlib.Resource
	children := map[string]childEntry{}
	objectKind := kindForObjectType(state.Type)
	lines, total := outputLines(out, config.maxResults)
	for _, line := range lines {
		objectName := nameAfterSlash(line)
		resources = append(resources, driverlib.Resource{Name: objectName, Kind: objectKind, Size: 0})
		children[objectName] = childEntry{isObject: true}
	}
	resources = appendTruncationNotice(resources, "objects", total, config.maxResults)
	return resources, children, nil
}

// kindForObjectType routes each object to the driver that understands its
// manifest best: well-known types get a dedicated kubernetes object driver,
// and everything else falls back to the generic JSON structure browser. The
// API group suffix (e.g. ".apps" in "deployments.apps") is ignored so the
// mapping works for any group.
func kindForObjectType(typeName string) string {
	base := typeName
	switch dot := strings.Index(typeName, "."); {
	case dot >= 0:
		base = typeName[:dot]
	default:
	}
	switch base {
	case "pods":
		return "k8s-pod"
	case "deployments", "statefulsets", "daemonsets", "replicasets", "jobs", "cronjobs":
		return "k8s-workload"
	case "services":
		return "k8s-service"
	case "configmaps", "secrets":
		return "k8s-configmap"
	case "nodes":
		return "k8s-node"
	default:
		return "json"
	}
}

// serveChildren answers range requests: container children get their mount
// state bytes for the next driver layer, object children get their manifest.
func serveChildren(enc *json.Encoder, dec *json.Decoder, config kubernetesConfig, state MountState, children map[string]childEntry) {
	for {
		var req driverlib.RangeReq
		switch err := dec.Decode(&req); {
		case err != nil:
			// EOF: the host closed our stdin, time to exit.
			return
		default:
		}
		switch {
		case req.Cmd != "range":
			continue
		default:
		}
		child, known := children[req.Child]
		switch {
		case req.Child == "" || !known:
			driverlib.SendRange(enc, nil)
		case child.isObject:
			driverlib.SendRange(enc, driverlib.SliceRange(fetchObjectManifest(config, state, req.Child), req.Offset, req.Length))
		default:
			driverlib.SendRange(enc, child.state)
		}
	}
}

// fetchObjectManifest reads one object's manifest bytes. Failures become a
// structured payload that every JSON and kubernetes presenter driver turns
// into a visible error resource.
func fetchObjectManifest(config kubernetesConfig, state MountState, objectName string) []byte {
	switch err := safePositional(state.Type, "kubernetes resource type"); {
	case err != nil:
		message := fmt.Sprintf("refusing to fetch the manifest of %q: %v", objectName, err)
		logKubernetesManifestFailure(message)
		return driverlib.EncodeDriverReadError(message)
	default:
	}
	switch err := safePositional(objectName, "kubernetes object name"); {
	case err != nil:
		message := fmt.Sprintf("refusing to fetch a kubernetes manifest: %v", err)
		logKubernetesManifestFailure(message)
		return driverlib.EncodeDriverReadError(message)
	default:
	}
	args := []string{"get", "--context=" + state.Context}
	switch {
	case state.Namespace != "":
		args = append(args, "--namespace="+state.Namespace)
	default:
	}
	// Serve the manifest as JSON so json_driver can browse it as a tree.
	args = append(args, state.Type, objectName, "-o", "json")
	out, err := runReadOnlyKubectl(config, args)
	switch {
	case err != nil:
		message := fmt.Sprintf("cannot fetch the manifest of %s %q: %v", state.Type, objectName, err)
		logKubernetesManifestFailure(message)
		return driverlib.EncodeDriverReadError(message)
	default:
		return out
	}
}

// runReadOnlyKubectl is the single point through which every kubectl
// invocation in this driver passes. It refuses anything outside the read-only
// allowlist by panicking before a process is spawned, runs kubectl directly
// with an argument vector (no shell), and returns kubectl's stdout.
func runReadOnlyKubectl(config kubernetesConfig, args []string) ([]byte, error) {
	assertReadOnlyInvocation(args)
	kubectlPath, resolveErr := resolveKubectl()
	switch {
	case resolveErr != nil:
		return nil, resolveErr
	default:
	}
	cmd := exec.Command(kubectlPath, args...)
	switch {
	case config.kubeconfig != "":
		cmd.Env = environmentWith("KUBECONFIG", config.kubeconfig)
	default:
	}
	stdout := boundedBuffer{limit: config.maxOutputBytes}
	stderr := boundedBuffer{limit: config.maxOutputBytes}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	switch {
	case stdout.exceeded():
		return nil, fmt.Errorf("kubectl %s produced %d stdout bytes while reading kubernetes data, exceeding DATATREE_KUBERNETES_MAX_OUTPUT_BYTES=%d", strings.Join(args, " "), stdout.total, config.maxOutputBytes)
	case stderr.exceeded():
		return nil, fmt.Errorf("kubectl %s produced %d stderr bytes while reading kubernetes data, exceeding DATATREE_KUBERNETES_MAX_OUTPUT_BYTES=%d", strings.Join(args, " "), stderr.total, config.maxOutputBytes)
	case runErr != nil && stdout.data.Len() > 0 && args[0] == "api-resources":
		// Partial success: kubectl can list most resources and still exit
		// non-zero when one API group is unavailable. Keep what we got.
		logPartialKubectlFailure(args, runErr, stderr.data.String())
		return stdout.data.Bytes(), nil
	case runErr != nil:
		return nil, fmt.Errorf("kubectl %s failed while reading kubernetes data: expected a zero exit, got %v with %d stdout bytes: %s", strings.Join(args, " "), runErr, stdout.total, strings.TrimSpace(stderr.data.String()))
	default:
		return stdout.data.Bytes(), nil
	}
}

// assertReadOnlyInvocation enforces the read-only guarantee. Only the
// subcommands in readOnlyKubectlCommands may run, and config is restricted to
// config get-contexts. Any other invocation is a programming error, so it
// panics with the full attempted command line.
func assertReadOnlyInvocation(args []string) {
	switch {
	case len(args) == 0:
		panic("kubectl invocation blocked: expected a subcommand, got no arguments at all")
	case !readOnlyKubectlCommands[args[0]]:
		panic(fmt.Sprintf("kubectl invocation blocked: subcommand %q is not in the read-only allowlist (get, api-resources, config get-contexts); refusing to run %q", args[0], strings.Join(args, " ")))
	case args[0] == "config" && (len(args) < 2 || args[1] != "get-contexts"):
		panic(fmt.Sprintf("kubectl invocation blocked: only \"config get-contexts\" is allowed under the config subcommand; refusing to run %q", strings.Join(args, " ")))
	default:
	}
}

// resolveKubectl locates the kubectl executable. DATATREE_KUBECTL overrides
// the search so tests can substitute a fake kubectl.
func resolveKubectl() (string, error) {
	candidate := os.Getenv("DATATREE_KUBECTL")
	switch {
	case candidate == "":
		candidate = "kubectl"
	default:
	}
	kubectlPath, err := exec.LookPath(candidate)
	switch {
	case err != nil:
		return "", fmt.Errorf("cannot find a usable kubectl executable: looked for %q (set DATATREE_KUBECTL to override), got: %v", candidate, err)
	default:
		return kubectlPath, nil
	}
}

func resolveKubernetesConfig() (kubernetesConfig, error) {
	maxResults, resultsErr := nonNegativeIntSetting("DATATREE_KUBERNETES_MAX_RESULTS", defaultKubernetesMaxResults)
	switch {
	case resultsErr != nil:
		return kubernetesConfig{}, resultsErr
	default:
	}
	maxOutputBytes, outputErr := positiveInt64Setting("DATATREE_KUBERNETES_MAX_OUTPUT_BYTES", defaultKubernetesMaxOutputBytes)
	switch {
	case outputErr != nil:
		return kubernetesConfig{}, outputErr
	default:
	}
	kubeconfig := os.Getenv("DATATREE_KUBECONFIG")
	switch err := validateSelection(kubeconfig, "DATATREE_KUBECONFIG"); {
	case err != nil:
		return kubernetesConfig{}, err
	default:
	}
	return kubernetesConfig{kubeconfig: kubeconfig, maxResults: maxResults, maxOutputBytes: maxOutputBytes}, nil
}

func nonNegativeIntSetting(name string, defaultValue int) (int, error) {
	raw := os.Getenv(name)
	switch {
	case raw == "":
		return defaultValue, nil
	default:
	}
	value, err := strconv.Atoi(raw)
	switch {
	case err != nil || value < 0:
		return 0, fmt.Errorf("cannot configure kubernetes browsing: expected %s to be a non-negative integer where 0 means unlimited, got %q", name, raw)
	default:
		return value, nil
	}
}

func positiveInt64Setting(name string, defaultValue int64) (int64, error) {
	raw := os.Getenv(name)
	switch {
	case raw == "":
		return defaultValue, nil
	default:
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	switch {
	case err != nil || value <= 0:
		return 0, fmt.Errorf("cannot configure kubernetes browsing: expected %s to be a positive byte count, got %q", name, raw)
	default:
		return value, nil
	}
}

func validateSelection(value string, name string) error {
	switch {
	case len(value) > maxKubernetesSelectionBytes:
		return fmt.Errorf("invalid kubernetes %s: received %d bytes, but selections are limited to %d bytes", name, len(value), maxKubernetesSelectionBytes)
	case strings.ContainsRune(value, '\x00'):
		return fmt.Errorf("invalid kubernetes %s %q: selections cannot contain a NUL byte", name, value)
	default:
		return nil
	}
}

func environmentWith(name string, value string) []string {
	prefix := name + "="
	environment := make([]string, 0, len(os.Environ())+1)
	for _, entry := range os.Environ() {
		switch {
		case strings.HasPrefix(entry, prefix):
			continue
		default:
			environment = append(environment, entry)
		}
	}
	return append(environment, prefix+value)
}

// safePositional rejects values that could be misread as kubectl flags or
// span multiple arguments. Kubernetes type and object names can never legally
// look like this, so no valid identifier is rejected.
func safePositional(value string, what string) error {
	switch {
	case value == "":
		return fmt.Errorf("cannot run kubectl: expected a non-empty %s", what)
	case len(value) > maxKubernetesSelectionBytes:
		return fmt.Errorf("cannot run kubectl: the %s is %d bytes, but selections are limited to %d bytes", what, len(value), maxKubernetesSelectionBytes)
	case strings.HasPrefix(value, "-"):
		return fmt.Errorf("cannot run kubectl: the %s %q starts with '-', which kubectl could misread as a flag", what, value)
	case strings.ContainsAny(value, " \t\r\n"):
		return fmt.Errorf("cannot run kubectl: the %s %q contains whitespace, which is never valid in a kubernetes identifier", what, value)
	default:
		return nil
	}
}

func encodeMountState(state MountState) ([]byte, error) {
	encoded, err := json.Marshal(state)
	switch {
	case err != nil:
		return nil, fmt.Errorf("cannot encode the kubernetes mount state for context %q namespace %q type %q: %v", state.Context, state.Namespace, state.Type, err)
	default:
		return encoded, nil
	}
}

func outputLines(out []byte, limit int) ([]string, int) {
	unique := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			continue
		default:
		}
		unique[trimmed] = true
	}
	lines := make([]string, 0, len(unique))
	for line := range unique {
		lines = append(lines, line)
	}
	sort.Strings(lines)
	total := len(lines)
	switch {
	case limit > 0 && len(lines) > limit:
		lines = lines[:limit]
	default:
	}
	return lines, total
}

func appendTruncationNotice(resources []driverlib.Resource, description string, total int, limit int) []driverlib.Resource {
	switch {
	case limit > 0 && total > limit:
		message := fmt.Sprintf("showing the first %d of %d %s; increase DATATREE_KUBERNETES_MAX_RESULTS to see more", limit, total, description)
		return append(resources, driverlib.Resource{Name: message, Kind: "error", Size: 0})
	default:
		return resources
	}
}

func (buffer *boundedBuffer) Write(data []byte) (int, error) {
	written := len(data)
	remaining := buffer.limit - int64(buffer.data.Len())
	switch {
	case remaining <= 0:
	case int64(len(data)) <= remaining:
		_, _ = buffer.data.Write(data)
	default:
		_, _ = buffer.data.Write(data[:remaining])
	}
	buffer.total += int64(written)
	return written, nil
}

func (buffer *boundedBuffer) exceeded() bool {
	return buffer.total > buffer.limit
}

// nameAfterSlash extracts the object name from kubectl -o name output such as
// "pod/nginx" or "namespace/default". Lines without a slash pass through.
func nameAfterSlash(line string) string {
	parts := strings.SplitN(line, "/", 2)
	switch {
	case len(parts) == 2:
		return parts[1]
	default:
		return line
	}
}
