// drivers_go/driverlib/driverlib.go
package driverlib

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// driverlib is the shared machinery for datatree drivers. It speaks the
// JSON-lines driver protocol, serves byte ranges from memory or from local
// files, browses generic JSON, classifies filesystem entries, and offers the
// small helpers the kubernetes presenter drivers use to build curated
// listings. Nothing here runs a subprocess or opens the network; the
// filesystem helpers only read local paths a driver explicitly passes in, so
// the read-only kubectl guarantee lives entirely in kubernetes_driver.

type RangeReq struct {
	Cmd    string `json:"cmd"`
	Offset int64  `json:"offset"`
	Length int64  `json:"length"`
	Child  string `json:"child,omitempty"`
}

type RangeResp struct {
	Data  string `json:"data"`
	Size  int64  `json:"size"`
	Error string `json:"error,omitempty"`
}

type Resource struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Size     int64  `json:"size"`
	Key      string `json:"key,omitempty"`
	Readable bool   `json:"readable,omitempty"`
	Payload  []byte `json:"-"`
}

type ResourcesMsg struct {
	Resources []Resource `json:"resources"`
}

type driverErrorEnvelope struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Message    string `json:"message"`
}

const driverErrorAPIVersion = "datatree.local/v1"
const driverErrorKind = "DriverReadError"

// ListFunc turns an upstream document into a resource listing plus the raw
// bytes of every servable child, keyed by that child's resource name.
type ListFunc func(raw []byte) ([]Resource, map[string][]byte, error)

// Run is the shared driver harness. It requests the whole upstream document,
// hands it to list, emits the resulting resources exactly once, then serves
// range requests for the servable children until the host closes stdin. A
// driver whose listing produced no servable children is a leaf: it exits
// immediately after emitting, matching the driver protocol's leaf rule.
func Run(name string, list ListFunc) {
	RunWithReader(name, WithSource(list), memoryChild)
}

// RunWithReader resolves lazy children through the ordinary byte protocol.
func RunWithReader(name string, list ListFunc, read func(string, []byte, int64, int64) ([]byte, error)) {
	defer RecoverPanic(name)

	enc := json.NewEncoder(os.Stdout)
	dec := json.NewDecoder(os.Stdin)

	raw, fetchErr := FetchUpstreamBytes(enc, dec)
	switch {
	case fetchErr != nil:
		fmt.Fprintf(os.Stderr, "%s cannot start: %v\n", name, fetchErr)
		EmitError(enc, fetchErr.Error())
		return
	default:
	}

	resources, children, listErr := list(raw)
	switch {
	case listErr != nil:
		EmitError(enc, listErr.Error())
		return
	default:
	}

	resources, children = BindPayloads(resources, children)
	for index := range resources {
		key := resources[index].Key
		switch {
		case key == "":
			key = resources[index].Name
		default:
		}
		switch resources[index].Kind {
		case "aws", "github", "directory", "git", "kubernetes":
			resources[index].Readable = false
		default:
			_, resources[index].Readable = children[key]
		}
	}
	switch err := enc.Encode(ResourcesMsg{Resources: resources}); {
	case err != nil:
		fmt.Fprintf(os.Stderr, "%s failed to encode %d resources: %v\n", name, len(resources), err)
		return
	default:
	}

	switch {
	case len(children) == 0:
		// Leaf listing: nothing is servable, so exit instead of lingering.
		return
	default:
		ServeChildrenWithReader(enc, dec, children, read)
	}
}

// RecoverPanic is the driver's safe point for panics: report the full
// detail on stderr and exit rather than crash silently.
func RecoverPanic(name string) {
	recovered := recover()
	switch {
	case recovered != nil:
		fmt.Fprintf(os.Stderr, "%s aborted: %v\n", name, recovered)
	default:
	}
}

// FetchUpstreamBytes asks the parent driver for the whole document.
func FetchUpstreamBytes(enc *json.Encoder, dec *json.Decoder) ([]byte, error) {
	switch err := enc.Encode(RangeReq{Cmd: "range", Offset: 0, Length: -1}); {
	case err != nil:
		return nil, fmt.Errorf("failed to request the document from the parent driver: %v", err)
	default:
	}
	var resp RangeResp
	switch err := dec.Decode(&resp); {
	case err != nil:
		return nil, fmt.Errorf("failed to decode the document response from the parent driver: %v", err)
	default:
	}
	switch {
	case resp.Error != "":
		return nil, fmt.Errorf("cannot read upstream data: %s", resp.Error)
	default:
	}
	raw, err := base64.StdEncoding.DecodeString(resp.Data)
	switch {
	case err != nil:
		return nil, fmt.Errorf("cannot decode the payload: expected base64 bytes from the parent driver, got decode error: %v", err)
	case int64(len(raw)) != resp.Size:
		return nil, fmt.Errorf("cannot read upstream data: response declared %d bytes but supplied %d", resp.Size, len(raw))
	default:
		return raw, nil
	}
}

// ServeChildren answers range requests by handing back the stored raw bytes of
// the requested child, honoring offset and length with -1 meaning "to the end".
func ServeChildren(enc *json.Encoder, dec *json.Decoder, children map[string][]byte) {
	ServeChildrenWithReader(enc, dec, children, func(_ string, value []byte, offset int64, length int64) ([]byte, error) {
		return SliceRange(value, offset, length), nil
	})
}

func ServeChildrenWithReader(enc *json.Encoder, dec *json.Decoder, children map[string][]byte, read func(string, []byte, int64, int64) ([]byte, error)) {
	for {
		var req RangeReq
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
		value, known := children[req.Child]
		switch {
		case req.Child == "" || !known:
			SendRange(enc, nil)
		default:
			data, err := read(req.Child, value, req.Offset, req.Length)
			switch {
			case err != nil:
				SendRangeError(enc, err.Error())
			default:
				SendRange(enc, data)
			}
		}
	}
}

// EmitError reports a failure as a single kind "error" resource so the browser
// shows the full message inside the tree.
func EmitError(enc *json.Encoder, message string) {
	fmt.Fprintf(os.Stderr, "%s\n", message)
	switch err := enc.Encode(ResourcesMsg{Resources: []Resource{{Name: message, Kind: "error", Size: 0}}}); {
	case err != nil:
		fmt.Fprintf(os.Stderr, "Failed to encode the error resource %q: %v\n", message, err)
	default:
	}
}

func EncodeDriverReadError(message string) []byte {
	encoded, err := json.Marshal(driverErrorEnvelope{APIVersion: driverErrorAPIVersion, Kind: driverErrorKind, Message: message})
	switch {
	case err != nil:
		panic(fmt.Sprintf("cannot encode driver read error %q: %v", message, err))
	default:
		return encoded
	}
}

func DecodeDriverReadError(raw []byte) (string, bool) {
	var envelope driverErrorEnvelope
	switch err := json.Unmarshal(raw, &envelope); {
	case err != nil:
		return "", false
	case envelope.APIVersion != driverErrorAPIVersion || envelope.Kind != driverErrorKind || envelope.Message == "":
		return "", false
	default:
		return envelope.Message, true
	}
}

func SendRange(enc *json.Encoder, data []byte) {
	resp := RangeResp{Data: base64.StdEncoding.EncodeToString(data), Size: int64(len(data))}
	switch err := enc.Encode(resp); {
	case err != nil:
		fmt.Fprintf(os.Stderr, "Failed to encode a range response of %d bytes: %v\n", len(data), err)
	default:
	}
}

// SliceRange applies the driver protocol's offset and length semantics to an
// in-memory byte slice. A negative length means "through the end".
func SliceRange(data []byte, offset int64, length int64) []byte {
	total := int64(len(data))
	switch {
	case offset < 0 || offset >= total:
		return nil
	default:
	}
	end := total
	// Compare via total-offset (never overflows: 0 <= offset < total here)
	// instead of offset+length, which can overflow int64 for a huge length.
	switch {
	case length >= 0 && length < total-offset:
		end = offset + length
	default:
	}
	return data[offset:end]
}

// ---- filesystem helpers shared by the directory-style drivers ----

// KindForName classifies a filesystem or archive entry by extension, matching
// the kind table in the spec so every driver routes files identically.
func KindForName(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".txt", ".md", ".log":
		return "text"
	case ".zip", ".tar", ".gz":
		return "archive"
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp":
		return "image"
	case ".pdf":
		return "pdf"
	case ".html", ".htm":
		return "html"
	case ".json":
		return "json"
	case ".csv":
		return "csv"
	default:
		return "file"
	}
}

// ListDirectory emits one resource per non-hidden entry of a directory. When
// the directory is inside a Git work tree, "." is exposed as kind git so the
// current repository can be expanded directly. Child Git work trees are also
// kind git, other directories are kind directory and size 0, and files are
// classified by extension with their byte size. Entries whose metadata cannot
// be read are skipped.
func ListDirectory(dirPath string) ([]Resource, error) {
	entries, err := os.ReadDir(dirPath)
	switch {
	case err != nil:
		return nil, fmt.Errorf("cannot list directory %s: expected a readable directory, got: %v", dirPath, err)
	default:
	}
	var resources []Resource
	switch {
	case InsideGitWorkTree(dirPath):
		resources = append(resources, Resource{Name: ".", Kind: "git", Size: 0})
	default:
	}
	for _, entry := range entries {
		name := entry.Name()
		// Skip hidden files
		switch {
		case strings.HasPrefix(name, "."):
			continue
		default:
		}
		info, infoErr := entry.Info()
		switch {
		case infoErr != nil:
			continue
		default:
		}
		switch {
		case info.IsDir():
			// Directories don't have meaningful size for our purposes
			kind := "directory"
			switch {
			case HasGitMarker(filepath.Join(dirPath, name)):
				kind = "git"
			default:
			}
			resources = append(resources, Resource{Name: name, Kind: kind, Size: 0})
		default:
			resources = append(resources, Resource{Name: name, Kind: KindForName(name), Size: info.Size(), Readable: true})
		}
	}
	return resources, nil
}

// HasGitMarker reports whether a directory has the .git directory or file used
// by normal work trees and linked work trees.
func HasGitMarker(dirPath string) bool {
	_, err := os.Stat(filepath.Join(dirPath, ".git"))
	switch {
	case err == nil:
		return true
	default:
		return false
	}
}

// InsideGitWorkTree reports whether dirPath is at or below a directory with a
// .git marker. It uses filesystem markers only; the Git driver still performs
// the authoritative repository validation when the user expands the row.
func InsideGitWorkTree(dirPath string) bool {
	current, err := filepath.Abs(dirPath)
	switch {
	case err != nil:
		return false
	default:
	}
	for {
		switch {
		case HasGitMarker(current):
			return true
		default:
		}
		parent := filepath.Dir(current)
		switch {
		case parent == current:
			return false
		default:
			current = parent
		}
	}
}

// InsideBase reports whether targetPath resolves inside basePath using
// path-segment-aware relative path checks, so "..", absolute escapes, and
// sibling prefixes are all rejected.
func InsideBase(basePath string, targetPath string) bool {
	rel, err := filepath.Rel(basePath, targetPath)
	switch {
	case err != nil:
		return false
	case rel == ".":
		return true
	case rel == "..":
		return false
	case strings.HasPrefix(rel, ".."+string(os.PathSeparator)):
		return false
	case filepath.IsAbs(rel):
		return false
	default:
		return true
	}
}

// ServeFileChild answers one range request for a filesystem child that has
// already passed the caller's InsideBase check. A directory child is served as
// its absolute path bytes (the payload directory_driver mounts on); a file
// child is served as the requested byte range. Every failure is served as an
// explicit protocol error so failures cannot masquerade as empty files.
func ServeFileChild(enc *json.Encoder, fullPath string, offset int64, length int64) {
	info, err := os.Stat(fullPath)
	switch {
	case err != nil:
		SendRangeError(enc, fmt.Sprintf("cannot stat file %q: %v", fullPath, err))
		return
	case info.IsDir():
		// Send directory path as bytes
		SendRange(enc, []byte(fullPath))
		return
	default:
	}
	file, err := os.Open(fullPath)
	switch {
	case err != nil:
		SendRangeError(enc, fmt.Sprintf("cannot open file %q: %v", fullPath, err))
		return
	default:
	}
	defer file.Close()
	switch _, seekErr := file.Seek(offset, 0); {
	case seekErr != nil:
		SendRangeError(enc, fmt.Sprintf("cannot seek file %q to byte %d: %v", fullPath, offset, seekErr))
		return
	default:
	}
	// Determine how much to read. Compare via size-offset (never overflows)
	// instead of offset+length, which can overflow int64 for a huge length.
	toRead := info.Size() - offset
	switch {
	case length >= 0 && length < toRead:
		// Don't read past EOF
		toRead = length
	default:
	}
	switch {
	case toRead <= 0:
		SendRange(enc, nil)
		return
	default:
	}
	data := make([]byte, toRead)
	n, readErr := io.ReadFull(file, data)
	switch {
	case readErr != nil && readErr != io.EOF:
		// A short read means the file changed underneath us; report the failed
		// range rather than bytes that no longer match the listed size.
		SendRangeError(enc, fmt.Sprintf("cannot read file %q at byte %d: expected %d bytes, got %d: %v", fullPath, offset, toRead, n, readErr))
	default:
		SendRange(enc, data[:n])
	}
}

// ---- generic JSON browsing ----

// ClassifyJSON reports whether a raw JSON value is an object, an array, or a
// scalar (string, number, boolean, or null) from its first non-space byte.
func ClassifyJSON(raw []byte) string {
	trimmed := strings.TrimSpace(string(raw))
	switch {
	case len(trimmed) == 0:
		return "scalar"
	case trimmed[0] == '{':
		return "object"
	case trimmed[0] == '[':
		return "array"
	default:
		return "scalar"
	}
}

// RenderScalar produces the display text for a scalar value: a JSON string is
// unquoted to its contents, and every other scalar shows its literal token.
// An empty string keeps its quotes so the row stays visible.
func RenderScalar(value json.RawMessage) string {
	trimmed := strings.TrimSpace(string(value))
	switch {
	case len(trimmed) > 0 && trimmed[0] == '"':
		var text string
		switch err := json.Unmarshal(value, &text); {
		case err != nil:
			return trimmed
		case text == "":
			return trimmed
		default:
			return text
		}
	default:
		return trimmed
	}
}

func TrimmedBytes(value json.RawMessage) []byte {
	return []byte(strings.TrimSpace(string(value)))
}

// KV builds a non-expandable leaf resource that displays "label: value".
func KV(label string, value string) Resource {
	return Resource{Name: label + ": " + value, Kind: "kv", Size: int64(len(value)), Payload: []byte(value)}
}

// Section describes an expandable "json" child for a raw JSON value under the
// given display name. It reports ok=false for a missing or scalar value, so a
// presenter can skip empty sections. The returned bytes are the child's
// servable payload.
func Section(name string, value json.RawMessage) (Resource, []byte, bool) {
	trimmed := TrimmedBytes(value)
	switch {
	case len(trimmed) == 0:
		return Resource{}, nil, false
	default:
	}
	switch ClassifyJSON(trimmed) {
	case "object", "array":
		return Resource{Name: name, Kind: "json", Size: int64(len(trimmed))}, trimmed, true
	default:
		return Resource{}, nil, false
	}
}

// TopFields parses a JSON object into its top-level fields as raw values,
// leaving each value unparsed so presenters can classify or serve it later.
func TopFields(raw []byte) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	switch err := json.Unmarshal(raw, &fields); {
	case err != nil:
		return nil, fmt.Errorf("cannot parse JSON object: expected a well-formed object, got %d bytes with error: %v", len(raw), err)
	default:
		return fields, nil
	}
}

// ListGenericJSON browses an arbitrary JSON document as a tree: objects list
// their keys (sorted for stable output), arrays list their elements as [i],
// and each object/array value becomes an expandable "json" child while each
// scalar becomes a "kv" leaf shown as "key: value". All values retain their
// serialized bytes for download and explicit filter composition.
func ListGenericJSON(raw []byte) ([]Resource, map[string][]byte, error) {

	switch ClassifyJSON(raw) {
	case "object":
		return listObjectFields(raw)
	case "array":
		return listArrayElements(raw)
	default:
		return listScalar(raw)
	}
}

func listObjectFields(raw []byte) ([]Resource, map[string][]byte, error) {
	fields, err := TopFields(raw)
	switch {
	case err != nil:
		return nil, nil, err
	default:
	}
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	// Sort keys so the tree is stable across expansions and self-test runs;
	// Go map iteration order is deliberately randomised.
	sort.Strings(names)
	var resources []Resource
	children := map[string][]byte{}
	for _, name := range names {
		resource, _ := DescribeChild(name, fields[name])
		resources = append(resources, resource)
		children[resource.Key] = TrimmedBytes(fields[name])
	}
	return resources, children, nil
}

func listArrayElements(raw []byte) ([]Resource, map[string][]byte, error) {
	var elements []json.RawMessage
	switch err := json.Unmarshal(raw, &elements); {
	case err != nil:
		return nil, nil, fmt.Errorf("cannot parse JSON array: expected a well-formed array, got %d bytes with error: %v", len(raw), err)
	default:
	}
	var resources []Resource
	children := map[string][]byte{}
	for index, value := range elements {
		key := "[" + strconv.Itoa(index) + "]"
		resource, _ := DescribeChild(key, value)
		resources = append(resources, resource)
		children[resource.Key] = TrimmedBytes(value)
	}
	return resources, children, nil
}

// listScalar handles a document that is itself a bare scalar. An empty
// document yields no resources, input that is not valid JSON is an error,
// and a valid scalar is shown as one leaf.
func listScalar(raw []byte) ([]Resource, map[string][]byte, error) {
	trimmed := TrimmedBytes(json.RawMessage(raw))
	switch {
	case len(trimmed) == 0:
		return nil, map[string][]byte{}, nil
	case !json.Valid(trimmed):
		return nil, nil, fmt.Errorf("cannot parse JSON document: expected an object, array, or scalar, got %d bytes that are not valid JSON: %.80q", len(raw), string(trimmed))
	default:
		return []Resource{{Name: RenderScalar(trimmed), Kind: "kv", Key: "\x00json:scalar", Size: int64(len(trimmed))}}, map[string][]byte{"\x00json:scalar": trimmed}, nil
	}
}

// DescribeChild turns one key/value pair into a resource. Object and array
// values become expandable "json" children whose raw bytes are servable;
// scalar values become non-expandable "kv" leaves that display "key: value".
func DescribeChild(key string, value json.RawMessage) (Resource, bool) {
	size := int64(len(TrimmedBytes(value)))
	internalKey := "\x00json:field:" + key
	switch ClassifyJSON(value) {
	case "object", "array":
		return Resource{Name: key, Key: internalKey, Kind: "json", Size: size}, true
	default:
		return Resource{Name: key + ": " + RenderScalar(value), Key: internalKey, Kind: "kv", Size: size}, false
	}
}

// ---- curated-view helpers shared by the presenter drivers ----

// AppendKV appends a "name: value" leaf, skipping absent values.
func AppendKV(resources []Resource, name string, value string) []Resource {
	switch {
	case value == "":
		return resources
	default:
		return append(resources, KV(name, value))
	}
}

// AppendCount appends a numeric leaf, skipping fields the manifest doesn't have.
func AppendCount(resources []Resource, name string, value *int64) []Resource {
	switch {
	case value == nil:
		return resources
	default:
		return AppendKV(resources, name, strconv.FormatInt(*value, 10))
	}
}

// AppendSection appends an expandable json child when the section has content.
func AppendSection(resources []Resource, children map[string][]byte, name string, value json.RawMessage) ([]Resource, map[string][]byte) {
	resource, payload, ok := Section(name, value)
	switch {
	case !ok:
		return resources, children
	default:
		resources = append(resources, resource)
		children[name] = payload
		return resources, children
	}
}

// RawObject decodes a raw JSON value as an object; anything else yields an
// empty map.
func RawObject(value json.RawMessage) map[string]json.RawMessage {
	switch {
	case len(TrimmedBytes(value)) == 0:
		return map[string]json.RawMessage{}
	default:
	}
	var fields map[string]json.RawMessage
	switch err := json.Unmarshal(value, &fields); {
	case err != nil:
		return map[string]json.RawMessage{}
	default:
		return fields
	}
}

func RawField(doc map[string]json.RawMessage, key string) json.RawMessage {
	value, ok := doc[key]
	switch {
	case ok:
		return value
	default:
		return nil
	}
}

// RawAtPath walks nested objects and returns the raw value at the path, or
// nil when any step is missing.
func RawAtPath(doc map[string]json.RawMessage, path ...string) json.RawMessage {
	current := doc
	for index, key := range path {
		value, ok := current[key]
		switch {
		case !ok:
			return nil
		case index == len(path)-1:
			return value
		default:
			current = RawObject(value)
		}
	}
	return nil
}

// SortedStringKeys returns a map's keys in stable sorted order, because Go
// map iteration order is deliberately randomised.
func SortedStringKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// ContainerSummary carries the two fields shown in a container's row name.
type ContainerSummary struct {
	Name  string `json:"name"`
	Image string `json:"image"`
}

// AppendContainers emits one expandable json child per container, named with
// the container's name, image, and (when known) runtime status, and serving
// the container's raw JSON so json_driver can browse the rest of it.
func AppendContainers(resources []Resource, children map[string][]byte, containersRaw json.RawMessage, statusByName map[string]string) ([]Resource, map[string][]byte) {
	var raws []json.RawMessage
	switch err := json.Unmarshal(containersRaw, &raws); {
	case err != nil:
		return resources, children
	default:
	}
	var summaries []ContainerSummary
	switch err := json.Unmarshal(containersRaw, &summaries); {
	case err != nil:
		return resources, children
	default:
	}
	for index, summary := range summaries {
		label := "container " + summary.Name + ": " + summary.Image
		detail, hasDetail := statusByName[summary.Name]
		switch {
		case hasDetail:
			label = label + " (" + detail + ")"
		default:
		}
		value := TrimmedBytes(raws[index])
		resources = append(resources, Resource{Name: label, Kind: "json", Size: int64(len(value))})
		children[label] = value
	}
	return resources, children
}
