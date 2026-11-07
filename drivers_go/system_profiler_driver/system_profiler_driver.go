package systemprofilerdriver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"datahumper/drivers_go/driverlib"
)

type systemProfilerConfig struct {
	executable     string
	maxOutputBytes int64
	timeout        time.Duration
}

type mountState struct {
	APIVersion string          `json:"apiVersion"`
	Profile    string          `json:"profile"`
	View       string          `json:"view"`
	Node       json.RawMessage `json:"node,omitempty"`
}

type boundedBuffer struct {
	data  bytes.Buffer
	limit int64
	total int64
}

const mountAPIVersion = "datatree.local/system-profiler/v1"
const profileHardware = "hardware"
const profileUSB = "usb"
const viewRoot = "root"
const viewNode = "node"
const hardwareDataType = "SPHardwareDataType"
const usbHostDataType = "SPUSBHostDataType"
const legacyUSBDataType = "SPUSBDataType"
const defaultSystemProfilerExecutable = "/usr/sbin/system_profiler"
const defaultSystemProfilerMaxOutputBytes int64 = 8 * 1024 * 1024
const defaultSystemProfilerTimeout = 30 * time.Second
const mountStateOverheadBytes int64 = 4096

const hardwareRootState = `{"apiVersion":"datatree.local/system-profiler/v1","profile":"hardware","view":"root"}`
const usbRootState = `{"apiVersion":"datatree.local/system-profiler/v1","profile":"usb","view":"root"}`

var systemProfilerFieldLabels = map[string]string{
	"Driver":                      "driver",
	"USBDeviceKeyLinkSpeed":       "link speed",
	"USBDeviceKeyPowerAllocation": "power allocation",
	"USBDeviceKeyProductID":       "product ID",
	"USBDeviceKeyProductVersion":  "product version",
	"USBDeviceKeySerialNumber":    "serial number",
	"USBDeviceKeyVendorID":        "vendor ID",
	"USBDeviceKeyVendorName":      "vendor",
	"USBKeyHardwareType":          "hardware type",
	"USBKeyLocationID":            "location ID",
	"activation_lock_status":      "activation lock status",
	"boot_rom_version":            "boot ROM version",
	"chip_type":                   "chip type",
	"machine_model":               "machine model",
	"machine_name":                "model name",
	"model_number":                "model number",
	"number_processors":           "processors",
	"os_loader_version":           "OS loader version",
	"physical_memory":             "physical memory",
	"platform_UUID":               "platform UUID",
	"provisioning_UDID":           "provisioning UDID",
	"serial_number":               "serial number",
}

func HardwareRootState() []byte {
	return []byte(hardwareRootState)
}

func USBRootState() []byte {
	return []byte(usbRootState)
}

func ResolveExecutable() (string, error) {
	candidate := os.Getenv("DATATREE_SYSTEM_PROFILER")
	switch {
	case candidate == "":
		candidate = defaultSystemProfilerExecutable
	default:
	}
	executable, err := exec.LookPath(candidate)
	switch {
	case err != nil:
		return "", fmt.Errorf("cannot find a usable system_profiler executable: looked for %q (set DATATREE_SYSTEM_PROFILER to override), got: %v", candidate, err)
	default:
		return executable, nil
	}
}

func Run() {
	driverlib.RunProvider("System profiler driver", buildSystemProfilerView)
}

func buildSystemProfilerView(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	var descriptor struct {
		APIVersion string `json:"apiVersion"`
	}
	switch err := json.Unmarshal(raw, &descriptor); {
	case err == nil && descriptor.APIVersion == mountAPIVersion:
		// Only root and legacy provider states use the navigation envelope.
	default:
		resources, children, err := listSystemProfilerValue(profileUSB, raw)
		switch {
		case err != nil:
			return nil, nil, err
		default:
			return driverlib.WithSource(func([]byte) ([]driverlib.Resource, map[string][]byte, error) { return resources, children, nil })(raw)
		}
	}
	config, configErr := resolveSystemProfilerConfig()
	switch {
	case configErr != nil:
		return nil, nil, configErr
	default:
	}
	state, stateErr := parseMountState(raw, config.maxOutputBytes)
	switch {
	case stateErr != nil:
		return nil, nil, stateErr
	default:
	}
	var value json.RawMessage
	switch state.View {
	case viewRoot:
		var readErr error
		value, readErr = readProfileRoot(config, state.Profile)
		switch {
		case readErr != nil:
			return nil, nil, readErr
		default:
		}
	case viewNode:
		value = driverlib.TrimmedBytes(state.Node)
	default:
		return nil, nil, fmt.Errorf("cannot browse local %s data: unsupported view %q", state.Profile, state.View)
	}
	return driverlib.WithSource(func([]byte) ([]driverlib.Resource, map[string][]byte, error) {
		return listSystemProfilerValue(state.Profile, value)
	})(value)
}

func resolveSystemProfilerConfig() (systemProfilerConfig, error) {
	executable, executableErr := ResolveExecutable()
	switch {
	case executableErr != nil:
		return systemProfilerConfig{}, executableErr
	default:
	}
	maxOutputBytes, settingErr := positiveByteSetting("DATATREE_SYSTEM_PROFILER_MAX_OUTPUT_BYTES", defaultSystemProfilerMaxOutputBytes)
	switch {
	case settingErr != nil:
		return systemProfilerConfig{}, settingErr
	default:
	}
	timeout, timeoutErr := positiveDurationSetting("DATATREE_SYSTEM_PROFILER_TIMEOUT", defaultSystemProfilerTimeout)
	switch {
	case timeoutErr != nil:
		return systemProfilerConfig{}, timeoutErr
	default:
		return systemProfilerConfig{executable: executable, maxOutputBytes: maxOutputBytes, timeout: timeout}, nil
	}
}

func parseMountState(raw []byte, maxOutputBytes int64) (mountState, error) {
	switch {
	case int64(len(raw)) > maxOutputBytes+mountStateOverheadBytes:
		return mountState{}, fmt.Errorf("cannot parse local system-profiler mount state: received %d bytes, expected at most %d bytes", len(raw), maxOutputBytes+mountStateOverheadBytes)
	default:
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var state mountState
	switch err := decoder.Decode(&state); {
	case err != nil:
		return mountState{}, fmt.Errorf("cannot parse local system-profiler mount state: expected a versioned JSON object, got %d bytes with error: %v", len(raw), err)
	default:
	}
	var extra json.RawMessage
	switch err := decoder.Decode(&extra); {
	case err == nil:
		return mountState{}, fmt.Errorf("cannot parse local system-profiler mount state: expected one JSON object, got trailing JSON data")
	case err != io.EOF:
		return mountState{}, fmt.Errorf("cannot parse local system-profiler mount state: expected one JSON object, got trailing data with error: %v", err)
	default:
	}
	return validateMountState(state)
}

func validateMountState(state mountState) (mountState, error) {
	switch {
	case state.APIVersion != mountAPIVersion:
		return mountState{}, fmt.Errorf("cannot browse local system-profiler data: expected apiVersion %q, got %q", mountAPIVersion, state.APIVersion)
	case state.Profile != profileHardware && state.Profile != profileUSB:
		return mountState{}, fmt.Errorf("cannot browse local system-profiler data: expected profile %q or %q, got %q", profileHardware, profileUSB, state.Profile)
	default:
	}
	switch state.View {
	case viewRoot:
		switch {
		case len(state.Node) != 0:
			return mountState{}, fmt.Errorf("cannot browse local %s root: expected no embedded node, got %d bytes", state.Profile, len(state.Node))
		default:
			return state, nil
		}
	case viewNode:
		switch {
		case len(driverlib.TrimmedBytes(state.Node)) == 0 || !json.Valid(state.Node):
			return mountState{}, fmt.Errorf("cannot browse local %s node: expected one valid JSON value, got %d bytes", state.Profile, len(state.Node))
		default:
			return state, nil
		}
	default:
		return mountState{}, fmt.Errorf("cannot browse local %s data: expected view %q or %q, got %q", state.Profile, viewRoot, viewNode, state.View)
	}
}

func readProfileRoot(config systemProfilerConfig, profile string) (json.RawMessage, error) {
	dataType, typeErr := dataTypeForProfile(config, profile)
	switch {
	case typeErr != nil:
		return nil, typeErr
	default:
	}
	out, runErr := runReadOnlySystemProfiler(config, []string{dataType, "-json"})
	switch {
	case runErr != nil:
		return nil, runErr
	default:
	}
	var document map[string]json.RawMessage
	switch err := json.Unmarshal(out, &document); {
	case err != nil:
		return nil, fmt.Errorf("cannot read local %s data: expected system_profiler %s -json to return a JSON object, got %d bytes with error: %v", profile, dataType, len(out), err)
	default:
	}
	value, found := document[dataType]
	switch {
	case !found:
		return nil, fmt.Errorf("cannot read local %s data: expected system_profiler JSON field %q, got %d top-level fields", profile, dataType, len(document))
	case profile == profileHardware:
		return flattenHardwareOverview(value)
	default:
		return driverlib.TrimmedBytes(value), nil
	}
}

func dataTypeForProfile(config systemProfilerConfig, profile string) (string, error) {
	switch profile {
	case profileHardware:
		return hardwareDataType, nil
	case profileUSB:
		return resolveUSBDataType(config)
	default:
		return "", fmt.Errorf("cannot select local system-profiler data: expected profile %q or %q, got %q", profileHardware, profileUSB, profile)
	}
}

func resolveUSBDataType(config systemProfilerConfig) (string, error) {
	out, err := runReadOnlySystemProfiler(config, []string{"-listDataTypes"})
	switch {
	case err != nil:
		return "", err
	default:
	}
	available := map[string]bool{}
	for _, field := range strings.Fields(string(out)) {
		available[field] = true
	}
	switch {
	case available[usbHostDataType]:
		return usbHostDataType, nil
	case available[legacyUSBDataType]:
		return legacyUSBDataType, nil
	default:
		return "", fmt.Errorf("cannot read local USB data: expected system_profiler -listDataTypes to contain %s or %s, got %d output bytes", usbHostDataType, legacyUSBDataType, len(out))
	}
}

func flattenHardwareOverview(value json.RawMessage) (json.RawMessage, error) {
	var entries []json.RawMessage
	switch err := json.Unmarshal(value, &entries); {
	case err != nil:
		return nil, fmt.Errorf("cannot read local hardware data: expected %s to be a JSON array, got %d bytes with error: %v", hardwareDataType, len(value), err)
	default:
	}
	switch len(entries) {
	case 1:
		return driverlib.TrimmedBytes(entries[0]), nil
	default:
		return driverlib.TrimmedBytes(value), nil
	}
}

func runReadOnlySystemProfiler(config systemProfilerConfig, args []string) ([]byte, error) {
	assertReadOnlySystemProfilerInvocation(args)
	ctx, cancel := context.WithTimeout(context.Background(), config.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, config.executable, args...)
	stdout := boundedBuffer{limit: config.maxOutputBytes}
	stderr := boundedBuffer{limit: config.maxOutputBytes}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	switch {
	case ctx.Err() == context.DeadlineExceeded:
		return nil, fmt.Errorf("system_profiler %s did not finish while reading local system-profiler data: expected completion within DATATREE_SYSTEM_PROFILER_TIMEOUT=%s", strings.Join(args, " "), config.timeout)
	case stdout.exceeded():
		return nil, fmt.Errorf("system_profiler %s produced %d stdout bytes while reading local system-profiler data, exceeding DATATREE_SYSTEM_PROFILER_MAX_OUTPUT_BYTES=%d", strings.Join(args, " "), stdout.total, config.maxOutputBytes)
	case stderr.exceeded():
		return nil, fmt.Errorf("system_profiler %s produced %d stderr bytes while reading local system-profiler data, exceeding DATATREE_SYSTEM_PROFILER_MAX_OUTPUT_BYTES=%d", strings.Join(args, " "), stderr.total, config.maxOutputBytes)
	case runErr != nil:
		return nil, fmt.Errorf("system_profiler %s failed while reading local system-profiler data: expected a zero exit, got %v with %d stdout bytes and stderr %q", strings.Join(args, " "), runErr, stdout.total, strings.TrimSpace(stderr.data.String()))
	default:
		return stdout.data.Bytes(), nil
	}
}

func assertReadOnlySystemProfilerInvocation(args []string) {
	switch {
	case stringSlicesEqual(args, []string{"-listDataTypes"}):
	case stringSlicesEqual(args, []string{hardwareDataType, "-json"}):
	case stringSlicesEqual(args, []string{usbHostDataType, "-json"}):
	case stringSlicesEqual(args, []string{legacyUSBDataType, "-json"}):
	default:
		panic(fmt.Sprintf("system_profiler invocation blocked: expected an exact read-only hardware, USB, or data-type-list command, refusing to run %q", strings.Join(args, " ")))
	}
}

func stringSlicesEqual(left []string, right []string) bool {
	switch {
	case len(left) != len(right):
		return false
	default:
	}
	for index := range left {
		switch {
		case left[index] != right[index]:
			return false
		default:
		}
	}
	return true
}

func listSystemProfilerValue(profile string, value json.RawMessage) ([]driverlib.Resource, map[string][]byte, error) {
	trimmed := driverlib.TrimmedBytes(value)
	switch {
	case !json.Valid(trimmed):
		return nil, nil, fmt.Errorf("cannot browse local %s data: expected valid JSON, got %d invalid bytes", profile, len(trimmed))
	default:
	}
	switch driverlib.ClassifyJSON(trimmed) {
	case "object":
		return listSystemProfilerObject(profile, trimmed)
	case "array":
		return listSystemProfilerArray(profile, trimmed)
	default:
		return []driverlib.Resource{{Name: driverlib.RenderScalar(trimmed), Kind: "kv", Size: int64(len(trimmed)), Payload: trimmed}}, map[string][]byte{}, nil
	}
}

func listSystemProfilerObject(profile string, value json.RawMessage) ([]driverlib.Resource, map[string][]byte, error) {
	fields, err := driverlib.TopFields(value)
	switch {
	case err != nil:
		return nil, nil, err
	default:
	}
	resources := []driverlib.Resource{}
	children := map[string][]byte{}
	usedNames := map[string]int{}
	items, hasItems := fields["_items"]
	switch {
	case hasItems:
		var itemsErr error
		resources, children, itemsErr = appendSystemProfilerItems(resources, children, usedNames, profile, items)
		switch {
		case itemsErr != nil:
			return nil, nil, itemsErr
		default:
		}
	default:
	}
	names := sortedVisibleFieldNames(fields)
	for _, name := range names {
		var appendErr error
		resources, children, appendErr = appendSystemProfilerValue(resources, children, usedNames, profile, displayFieldName(name), fields[name])
		switch {
		case appendErr != nil:
			return nil, nil, appendErr
		default:
		}
	}
	return resources, children, nil
}

func appendSystemProfilerItems(resources []driverlib.Resource, children map[string][]byte, usedNames map[string]int, profile string, value json.RawMessage) ([]driverlib.Resource, map[string][]byte, error) {
	var items []json.RawMessage
	switch err := json.Unmarshal(value, &items); {
	case err != nil:
		return nil, nil, fmt.Errorf("cannot browse local %s data: expected _items to be a JSON array, got %d bytes with error: %v", profile, len(value), err)
	default:
	}
	for index, item := range items {
		label, childValue, labelErr := describeArrayItem(index, item)
		switch {
		case labelErr != nil:
			return nil, nil, labelErr
		default:
		}
		var appendErr error
		resources, children, appendErr = appendSystemProfilerValue(resources, children, usedNames, profile, label, childValue)
		switch {
		case appendErr != nil:
			return nil, nil, appendErr
		default:
		}
	}
	return resources, children, nil
}

func listSystemProfilerArray(profile string, value json.RawMessage) ([]driverlib.Resource, map[string][]byte, error) {
	var items []json.RawMessage
	switch err := json.Unmarshal(value, &items); {
	case err != nil:
		return nil, nil, fmt.Errorf("cannot browse local %s data: expected a JSON array, got %d bytes with error: %v", profile, len(value), err)
	default:
	}
	resources := []driverlib.Resource{}
	children := map[string][]byte{}
	usedNames := map[string]int{}
	return appendSystemProfilerItems(resources, children, usedNames, profile, value)
}

func describeArrayItem(index int, value json.RawMessage) (string, json.RawMessage, error) {
	trimmed := driverlib.TrimmedBytes(value)
	switch driverlib.ClassifyJSON(trimmed) {
	case "object":
		fields, err := driverlib.TopFields(trimmed)
		switch {
		case err != nil:
			return "", nil, err
		default:
		}
		nameValue, hasName := fields["_name"]
		switch {
		case hasName:
			name, nameErr := systemProfilerName(nameValue)
			switch {
			case nameErr != nil:
				return "", nil, nameErr
			case name != "":
				return name, trimmed, nil
			default:
			}
		case len(fields) == 1:
			return unwrapSingleField(fields, trimmed)
		default:
		}
	default:
	}
	return "[" + strconv.Itoa(index) + "]", trimmed, nil
}

func unwrapSingleField(fields map[string]json.RawMessage, original json.RawMessage) (string, json.RawMessage, error) {
	for name, value := range fields {
		switch driverlib.ClassifyJSON(value) {
		case "object", "array":
			return displayFieldName(name), driverlib.TrimmedBytes(value), nil
		default:
			return displayFieldName(name), original, nil
		}
	}
	return "", nil, fmt.Errorf("cannot name local system-profiler object: expected one field, got none")
}

func appendSystemProfilerValue(resources []driverlib.Resource, children map[string][]byte, usedNames map[string]int, profile string, label string, value json.RawMessage) ([]driverlib.Resource, map[string][]byte, error) {
	trimmed := driverlib.TrimmedBytes(value)
	name := uniqueResourceName(label, usedNames)
	switch driverlib.ClassifyJSON(trimmed) {
	case "object", "array":
		resources = append(resources, driverlib.Resource{Name: name, Kind: profile, Size: int64(len(trimmed)), Readable: true})
		children[name] = trimmed
		return resources, children, nil
	default:
		resources = append(resources, driverlib.Resource{Name: name + ": " + driverlib.RenderScalar(trimmed), Kind: "kv", Size: int64(len(trimmed)), Payload: trimmed})
		return resources, children, nil
	}
}

func encodeNodeState(profile string, node json.RawMessage) ([]byte, error) {
	state := mountState{APIVersion: mountAPIVersion, Profile: profile, View: viewNode, Node: driverlib.TrimmedBytes(node)}
	encoded, err := json.Marshal(state)
	switch {
	case err != nil:
		return nil, fmt.Errorf("cannot encode local %s child containing %d JSON bytes: %v", profile, len(node), err)
	default:
		return encoded, nil
	}
}

func sortedVisibleFieldNames(fields map[string]json.RawMessage) []string {
	names := make([]string, 0, len(fields))
	for name := range fields {
		switch name {
		case "_items", "_name":
		default:
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func systemProfilerName(value json.RawMessage) (string, error) {
	var name string
	switch err := json.Unmarshal(value, &name); {
	case err != nil:
		return "", fmt.Errorf("cannot browse local system-profiler data: expected _name to be a string, got %q with error: %v", string(value), err)
	case strings.TrimSpace(name) == "":
		return "", nil
	default:
		return humanizeIdentifier(name), nil
	}
}

func displayFieldName(name string) string {
	label, known := systemProfilerFieldLabels[name]
	switch {
	case known:
		return label
	default:
		return humanizeIdentifier(name)
	}
}

func humanizeIdentifier(value string) string {
	normalized := strings.NewReplacer("_", " ", "-", " ").Replace(strings.TrimSpace(value))
	runes := []rune(normalized)
	var builder strings.Builder
	for index, current := range runes {
		switch {
		case index > 0 && needsWordBoundary(runes, index):
			builder.WriteRune(' ')
		default:
		}
		builder.WriteRune(current)
	}
	return strings.Join(strings.Fields(builder.String()), " ")
}

func needsWordBoundary(value []rune, index int) bool {
	current := value[index]
	previous := value[index-1]
	var next rune
	switch {
	case index+1 < len(value):
		next = value[index+1]
	default:
	}
	switch {
	case !unicode.IsUpper(current) || unicode.IsSpace(previous):
		return false
	case unicode.IsLower(previous) || unicode.IsDigit(previous):
		return true
	case next != 0 && unicode.IsLower(next) && unicode.IsUpper(previous):
		return true
	default:
		return false
	}
}

func uniqueResourceName(name string, used map[string]int) string {
	count := used[name] + 1
	used[name] = count
	switch count {
	case 1:
		return name
	default:
		return fmt.Sprintf("%s [%d]", name, count)
	}
}

func positiveByteSetting(name string, defaultValue int64) (int64, error) {
	raw := os.Getenv(name)
	switch {
	case raw == "":
		return defaultValue, nil
	default:
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	switch {
	case err != nil || value <= 0 || value > math.MaxInt64-mountStateOverheadBytes:
		return 0, fmt.Errorf("cannot configure local hardware browsing: expected %s to be a positive byte count that leaves room for mount-state metadata, got %q", name, raw)
	default:
		return value, nil
	}
}

func positiveDurationSetting(name string, defaultValue time.Duration) (time.Duration, error) {
	raw := os.Getenv(name)
	switch {
	case raw == "":
		return defaultValue, nil
	default:
	}
	value, err := time.ParseDuration(raw)
	switch {
	case err != nil || value <= 0:
		return 0, fmt.Errorf("cannot configure local hardware browsing: expected %s to be a positive duration such as 30s, got %q", name, raw)
	default:
		return value, nil
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
