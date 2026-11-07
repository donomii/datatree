// main.go
package main

import (
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	awsdriver "datahumper/drivers_go/aws_driver"
	csvdriver "datahumper/drivers_go/csv_driver"
	diffdriver "datahumper/drivers_go/diff_driver"
	directorydriver "datahumper/drivers_go/directory_driver"
	gitdriver "datahumper/drivers_go/git_driver"
	githubdriver "datahumper/drivers_go/github_driver"
	htmldriver "datahumper/drivers_go/html_driver"
	imagedriver "datahumper/drivers_go/image_driver"
	jsondriver "datahumper/drivers_go/json_driver"
	k8sconfigmapdriver "datahumper/drivers_go/k8s_configmap_driver"
	k8snodedriver "datahumper/drivers_go/k8s_node_driver"
	k8spoddriver "datahumper/drivers_go/k8s_pod_driver"
	k8sservicedriver "datahumper/drivers_go/k8s_service_driver"
	k8sworkloaddriver "datahumper/drivers_go/k8s_workload_driver"
	kubernetesdriver "datahumper/drivers_go/kubernetes_driver"
	pdfdriver "datahumper/drivers_go/pdf_driver"
	rootdriver "datahumper/drivers_go/root_driver"
	systemprofilerdriver "datahumper/drivers_go/system_profiler_driver"
	textdriver "datahumper/drivers_go/text_driver"
	zipdriver "datahumper/drivers_go/zip_driver"
)

const defaultAddr = "127.0.0.1:0"

//go:embed web/index.html
var webIndex []byte

var driverNames = []string{"root_driver", "directory_driver", "git_driver", "github_driver", "aws_driver", "system_profiler_driver", "diff_driver", "zip_driver", "text_driver", "kubernetes_driver", "json_driver", "k8s_pod_driver", "k8s_workload_driver", "k8s_service_driver", "k8s_configmap_driver", "k8s_node_driver", "image_driver", "pdf_driver", "html_driver", "csv_driver"}

var uploadDriverNames = []string{"diff_driver", "zip_driver", "text_driver", "json_driver", "image_driver", "pdf_driver", "html_driver", "csv_driver"}

// defaultIdleTimeout is how long a driver session may go without serving a
// request before the idle sweep stops it. DATATREE_IDLE_TIMEOUT overrides it.
const defaultIdleTimeout = 30 * time.Minute

const defaultMaxUploadBytes int64 = 64 * 1024 * 1024
const defaultMaxRetainedUploadBytes int64 = 256 * 1024 * 1024
const defaultUploadIdleTimeout = 30 * time.Minute

// idleSweepInterval is how often the idle sweep looks for stale sessions.
const idleSweepInterval = time.Minute

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
	Handle   string `json:"handle,omitempty"`
	Readable bool   `json:"readable,omitempty"`
}

type ResourcesMsg struct {
	Resources []Resource `json:"resources"`
}

type DriverMsg struct {
	Cmd       string     `json:"cmd,omitempty"`
	Offset    int64      `json:"offset,omitempty"`
	Length    int64      `json:"length,omitempty"`
	Child     string     `json:"child,omitempty"`
	Resources []Resource `json:"resources"`
}

type ListResponse struct {
	Resources []Resource `json:"resources"`
}

type UploadResponse struct {
	ResID string `json:"res_id"`
	Size  int    `json:"size"`
}

type UploadSettingsResponse struct {
	MaxUploadBytes         int64  `json:"max_upload_bytes"`
	MaxRetainedUploadBytes int64  `json:"max_retained_upload_bytes"`
	UploadIdleTimeout      string `json:"upload_idle_timeout"`
	Explanation            string `json:"explanation"`
}

type LoadResponse struct {
	Resources []Resource `json:"resources"`
	Driver    string     `json:"driver"`
	Session   string     `json:"session"`
}

type UnloadResponse struct {
	Session string `json:"session"`
	Stopped bool   `json:"stopped"`
}

type GitHubActionRequest struct {
	Handle      string `json:"handle"`
	Body        string `json:"body,omitempty"`
	MergeMethod string `json:"merge_method,omitempty"`
}

type GitHubActionResponse struct {
	Message string `json:"message"`
}

type AWSActionRequest struct {
	Handle string `json:"handle"`
	Body   string `json:"body,omitempty"`
}

type AWSActionResponse struct {
	Message string `json:"message"`
}

type driverProc struct {
	cmd    *exec.Cmd
	enc    *json.Encoder
	dec    *json.Decoder
	stdin  io.WriteCloser
	stdout io.ReadCloser
	mu     sync.Mutex

	// lastUsed is guarded by its own lock (not mu) because it is touched
	// inside requestRange while mu is already held.
	lastUsedMu sync.Mutex
	lastUsed   time.Time
}

type uploadedBlob struct {
	data     []byte
	lastUsed time.Time
}

func (p *driverProc) touch() {
	p.lastUsedMu.Lock()
	defer p.lastUsedMu.Unlock()
	p.lastUsed = time.Now()
}

func (p *driverProc) lastUsedAt() time.Time {
	p.lastUsedMu.Lock()
	defer p.lastUsedMu.Unlock()
	return p.lastUsed
}

func startDriver(driverName string, args ...string) (*driverProc, error) {
	executablePath, err := os.Executable()
	switch {
	case err != nil:
		return nil, fmt.Errorf("cannot start embedded driver %s: expected the current Datatree executable path, got error: %v", driverName, err)
	default:
	}
	driverArgs := append([]string{"--driver", driverName}, args...)
	cmd := exec.Command(executablePath, driverArgs...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &driverProc{
		cmd:      cmd,
		enc:      json.NewEncoder(stdin),
		dec:      json.NewDecoder(stdout),
		stdin:    stdin,
		stdout:   stdout,
		lastUsed: time.Now(),
	}, nil
}

func (p *driverProc) stop() {
	_ = p.stdin.Close()
	_ = p.cmd.Wait()
}

func (p *driverProc) requestRange(req RangeReq) (RangeResp, error) {
	p.touch()
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.enc.Encode(req); err != nil {
		return RangeResp{}, err
	}
	var resp RangeResp
	if err := p.dec.Decode(&resp); err != nil {
		return RangeResp{}, err
	}
	return resp, nil
}

// ---------------- Store / Router ----------------

type Store struct {
	recoveryKey            []byte
	origins                map[string]routeOrigin
	recoveryGate           chan struct{}
	mu                     sync.RWMutex
	blobs                  map[string]uploadedBlob
	blobBytes              int64
	maxUploadBytes         int64
	maxRetainedUploadBytes int64
	uploadIdleTimeout      time.Duration
	driverBlobIDs          map[string]string

	// handles (child resources): hid -> (childName, size, kind)
	handles map[string]ChildHandle

	rootDirectory string

	// driver processes by name
	drivers map[string]*driverProc

	// root resources
	rootResources []Resource
}

type ChildHandle struct {
	Name       string
	Route      *handleRoute
	Hid        string
	ChildName  string
	Size       int64
	Kind       string
	DriverName string
}

func NewStore() *Store {
	return NewStoreWithUploadLimits(defaultMaxUploadBytes, defaultMaxRetainedUploadBytes, defaultUploadIdleTimeout)
}

func NewStoreWithUploadLimits(maxUploadBytes int64, maxRetainedUploadBytes int64, uploadIdleTimeout time.Duration) *Store {
	key := make([]byte, 32)
	switch _, err := rand.Read(key); {
	case err != nil:
		panic(fmt.Sprintf("cannot initialize handle recovery key: %v", err))
	default:
	}
	gate := make(chan struct{}, 1)
	gate <- struct{}{}
	return &Store{
		recoveryKey: key, origins: map[string]routeOrigin{}, recoveryGate: gate,
		blobs:                  map[string]uploadedBlob{},
		handles:                map[string]ChildHandle{},
		drivers:                map[string]*driverProc{},
		maxUploadBytes:         maxUploadBytes,
		maxRetainedUploadBytes: maxRetainedUploadBytes,
		uploadIdleTimeout:      uploadIdleTimeout,
		driverBlobIDs:          map[string]string{},
	}
}

func resolveListenAddr() string {
	addr := os.Getenv("DATATREE_ADDR")
	if addr != "" {
		return addr
	}
	return defaultAddr
}

func resolveRootDirectory(args []string) (string, error) {
	var requestedDirectory string
	switch len(args) {
	case 0:
		return "", nil
	case 1:
		requestedDirectory = args[0]
	default:
		return "", fmt.Errorf("cannot select the directory to open: expected zero or one directory argument, got %d arguments: %q", len(args), args)
	}
	absoluteDirectory, err := filepath.Abs(requestedDirectory)
	switch {
	case err != nil:
		return "", fmt.Errorf("cannot open directory %q: expected an absolute path, got path resolution error: %v", requestedDirectory, err)
	default:
	}
	info, err := os.Stat(absoluteDirectory)
	switch {
	case err != nil:
		return "", fmt.Errorf("cannot open directory %q: expected an existing readable directory, got stat error: %v", absoluteDirectory, err)
	case !info.IsDir():
		return "", fmt.Errorf("cannot open directory %q: expected a directory, got filesystem mode %s", absoluteDirectory, info.Mode())
	default:
		return absoluteDirectory, nil
	}
}

func browserURL(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	switch {
	case err != nil:
		return "http://" + addr
	case host == "" || host == "0.0.0.0" || host == "::":
		return "http://localhost:" + port
	default:
		return "http://" + net.JoinHostPort(host, port)
	}
}

// resolveIdleTimeout reads DATATREE_IDLE_TIMEOUT as a Go duration string such
// as "45m" or "2h". Zero or a negative duration disables the idle sweep. An
// unparseable value is a fatal configuration error rather than a silent
// fallback.
func resolveIdleTimeout() time.Duration {
	raw := os.Getenv("DATATREE_IDLE_TIMEOUT")
	switch {
	case raw == "":
		return defaultIdleTimeout
	default:
	}
	timeout, err := time.ParseDuration(raw)
	switch {
	case err != nil:
		log.Fatalf("cannot configure the idle-session cleanup: expected DATATREE_IDLE_TIMEOUT to be a Go duration such as \"30m\" or \"0\" to disable it, got %q with error: %v", raw, err)
		return defaultIdleTimeout
	default:
		return timeout
	}
}

func resolvePositiveByteLimit(envName string, defaultValue int64) int64 {
	raw := os.Getenv(envName)
	switch {
	case raw == "":
		return defaultValue
	default:
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	switch {
	case err != nil || value <= 0:
		log.Fatalf("cannot configure upload retention: expected %s to be a positive byte count, got %q", envName, raw)
		return defaultValue
	default:
		return value
	}
}

func resolveUploadIdleTimeout() time.Duration {
	raw := os.Getenv("DATATREE_UPLOAD_IDLE_TIMEOUT")
	switch {
	case raw == "":
		return defaultUploadIdleTimeout
	default:
	}
	timeout, err := time.ParseDuration(raw)
	switch {
	case err != nil || timeout < 0:
		log.Fatalf("cannot configure upload retention: expected DATATREE_UPLOAD_IDLE_TIMEOUT to be a non-negative Go duration such as \"30m\" or \"0\" to disable expiry, got %q", raw)
		return defaultUploadIdleTimeout
	default:
		return timeout
	}
}

func newConfiguredStore() *Store {
	maxUploadBytes := resolvePositiveByteLimit("DATATREE_MAX_UPLOAD_BYTES", defaultMaxUploadBytes)
	maxRetainedUploadBytes := resolvePositiveByteLimit("DATATREE_MAX_RETAINED_UPLOAD_BYTES", defaultMaxRetainedUploadBytes)
	switch {
	case maxUploadBytes > maxRetainedUploadBytes:
		log.Fatalf("cannot configure upload retention: DATATREE_MAX_UPLOAD_BYTES is %d bytes, but DATATREE_MAX_RETAINED_UPLOAD_BYTES is only %d bytes", maxUploadBytes, maxRetainedUploadBytes)
		return NewStore()
	default:
		return NewStoreWithUploadLimits(maxUploadBytes, maxRetainedUploadBytes, resolveUploadIdleTimeout())
	}
}

func (s *Store) blobInUseLocked(id string) bool {
	for _, blobID := range s.driverBlobIDs {
		switch {
		case blobID == id:
			return true
		default:
		}
	}
	return false
}

func (s *Store) removeBlobLocked(id string) {
	blob, ok := s.blobs[id]
	switch {
	case ok:
		delete(s.blobs, id)
		s.blobBytes -= int64(len(blob.data))
	default:
	}
}

func (s *Store) reapIdleBlobsLocked(now time.Time) []string {
	reaped := []string{}
	switch {
	case s.uploadIdleTimeout <= 0:
		return reaped
	default:
	}
	cutoff := now.Add(-s.uploadIdleTimeout)
	for id, blob := range s.blobs {
		switch {
		case s.blobInUseLocked(id):
		case blob.lastUsed.After(cutoff):
		default:
			s.removeBlobLocked(id)
			reaped = append(reaped, id)
		}
	}
	return reaped
}

func (s *Store) reapIdleBlobs(now time.Time) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reapIdleBlobsLocked(now)
}

func (s *Store) evictOldestUnusedBlobLocked() bool {
	oldestID := ""
	oldestTime := time.Time{}
	for id, blob := range s.blobs {
		switch {
		case s.blobInUseLocked(id):
		case oldestID == "" || blob.lastUsed.Before(oldestTime):
			oldestID = id
			oldestTime = blob.lastUsed
		default:
		}
	}
	switch {
	case oldestID == "":
		return false
	default:
		s.removeBlobLocked(oldestID)
		return true
	}
}

func (s *Store) putBlob(b []byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reapIdleBlobsLocked(time.Now())
	size := int64(len(b))
	switch {
	case size > s.maxUploadBytes:
		return "", fmt.Errorf("upload is %d bytes, but DATATREE_MAX_UPLOAD_BYTES allows at most %d bytes", size, s.maxUploadBytes)
	default:
	}
	for s.blobBytes+size > s.maxRetainedUploadBytes {
		switch {
		case s.evictOldestUnusedBlobLocked():
		default:
			return "", fmt.Errorf("upload needs %d bytes with %d bytes already retained, but DATATREE_MAX_RETAINED_UPLOAD_BYTES allows at most %d bytes and every retained upload is in use", size, s.blobBytes, s.maxRetainedUploadBytes)
		}
	}
	id := newID("res")
	s.blobs[id] = uploadedBlob{data: b, lastUsed: time.Now()}
	s.blobBytes += size
	return id, nil
}

func (s *Store) getBlob(id string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	blob, ok := s.blobs[id]
	switch {
	case ok:
		blob.lastUsed = time.Now()
		s.blobs[id] = blob
		return blob.data, true
	default:
		return nil, false
	}
}

func (s *Store) reserveBlobForDriver(driverName string, blobID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	blob, ok := s.blobs[blobID]
	switch {
	case ok:
		blob.lastUsed = time.Now()
		s.blobs[blobID] = blob
		s.driverBlobIDs[driverName] = blobID
		return true
	default:
		return false
	}
}

func (s *Store) releaseBlobReservation(driverName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.driverBlobIDs, driverName)
}

func (s *Store) addHandle(h ChildHandle) string {
	h = s.recoverableHandle(h)
	s.mu.Lock()
	defer s.mu.Unlock()
	if h.Hid == "" {
		h.Hid = newID("hdl")
	}
	s.handles[h.Hid] = h
	return h.Hid
}

func (s *Store) getHandle(hid string) (ChildHandle, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	h, ok := s.handles[hid]
	return h, ok
}

func (s *Store) getDriver(name string) (*driverProc, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.drivers[name]
	return d, ok
}

func (s *Store) addDriver(name string, proc *driverProc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.drivers[name] = proc
}

func (s *Store) takeDriver(name string, retireBlob bool) (*driverProc, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	proc, ok := s.drivers[name]
	switch {
	case ok:
		delete(s.drivers, name)
		delete(s.origins, name)
		for handleID, handle := range s.handles {
			switch {
			case handle.DriverName == name:
				delete(s.handles, handleID)
			default:
			}
		}
		blobID, hasBlob := s.driverBlobIDs[name]
		delete(s.driverBlobIDs, name)
		switch {
		case retireBlob && hasBlob:
			s.removeBlobLocked(blobID)
		default:
		}
	default:
	}
	return proc, ok
}

func (s *Store) stopDriver(name string) bool {
	return s.stopDriverWithBlobPolicy(name, true)
}

func (s *Store) stopDriverWithBlobPolicy(name string, retireBlob bool) bool {
	proc, ok := s.takeDriver(name, retireBlob)
	switch {
	case ok:
		proc.stop()
	default:
	}
	return ok
}

func (s *Store) takeDrivers() map[string]*driverProc {
	s.mu.Lock()
	defer s.mu.Unlock()
	drivers := s.drivers
	s.drivers = map[string]*driverProc{}
	s.handles = map[string]ChildHandle{}
	s.blobs = map[string]uploadedBlob{}
	s.blobBytes = 0
	s.driverBlobIDs = map[string]string{}
	s.origins = map[string]routeOrigin{}
	return drivers
}

func (s *Store) stopDrivers() {
	drivers := s.takeDrivers()
	for _, proc := range drivers {
		proc.stop()
	}
}

func (s *Store) snapshotDrivers() map[string]*driverProc {
	s.mu.RLock()
	defer s.mu.RUnlock()
	drivers := map[string]*driverProc{}
	for name, proc := range s.drivers {
		drivers[name] = proc
	}
	return drivers
}

// reapIdleDrivers stops every driver session that has served no request for at
// least idleFor and returns the stopped session names. The root_driver session
// is exempt because every selected-directory expansion is served through it; a
// reaped session's subtree simply re-expands from its parent on the next
// click, which restarts the driver.
func (s *Store) reapIdleDrivers(idleFor time.Duration) []string {
	cutoff := time.Now().Add(-idleFor)
	reaped := []string{}
	for name, proc := range s.snapshotDrivers() {
		switch {
		case name == "root_driver":
			continue
		case proc.lastUsedAt().After(cutoff):
			continue
		default:
			s.stopDriver(name)
			reaped = append(reaped, name)
		}
	}
	return reaped
}

func newID(prefix string) string {
	return prefix + "-" + uuid.NewString()
}

// Range upstream (root blob)
func (s *Store) serveBlobRange(blobID string, off, length int64) (chunk []byte) {
	b, ok := s.getBlob(blobID)
	if !ok {
		return nil
	}
	start := off
	end := int64(len(b))
	if length >= 0 && off+length < end {
		end = off + length
	}
	if start < 0 || start > int64(len(b)) {
		return nil
	}
	return b[start:end]
}

// Range via driver for a child
func (s *Store) serveLiveChildRange(h ChildHandle, off, length int64) ([]byte, error) {
	driver, ok := s.getDriver(h.DriverName)
	if !ok {
		return nil, fmt.Errorf("cannot serve range for handle %s child %s: driver %s is not running", h.Hid, h.ChildName, h.DriverName)
	}
	req := RangeReq{Cmd: "range", Offset: off, Length: length, Child: h.ChildName}
	resp, err := driver.requestRange(req)
	if err != nil {
		return nil, fmt.Errorf("cannot serve range for handle %s child %s from driver %s: %v", h.Hid, h.ChildName, h.DriverName, err)
	}
	switch {
	case resp.Error != "":
		return nil, fmt.Errorf("cannot read child %q: %s", h.ChildName, resp.Error)
	default:
	}
	if resp.Size == 0 && resp.Data == "" {
		return []byte{}, nil
	}
	raw, err := base64.StdEncoding.DecodeString(resp.Data)
	if err != nil {
		return nil, err
	}
	switch {
	case int64(len(raw)) != resp.Size:
		return nil, fmt.Errorf("invalid child range for %q: expected %d bytes, got %d", h.ChildName, resp.Size, len(raw))
	default:
	}
	return raw, nil
}

// Load root driver and return resource list
func (s *Store) loadRootDriver() (resources []Resource, err error) {
	// Stop existing root driver if any
	if _, ok := s.getDriver("root_driver"); ok {
		s.stopDriverWithBlobPolicy("root_driver", false)
	}
	s.setOrigin("root_driver", routeOrigin{Route: handleRoute{Version: 1, Root: s.rootDirectory}, Driver: "root_driver"})

	var proc *driverProc
	switch {
	case s.rootDirectory == "":
		proc, err = startDriver("root_driver")
	default:
		proc, err = startDriver("root_driver", s.rootDirectory)
	}
	if err != nil {
		return nil, err
	}
	s.addDriver("root_driver", proc)

	// DRIVER INIT FLOW:
	// 1) Driver asks us for bytes (range).
	// 2) We feed from root blob.
	// 3) Driver emits resources once, we capture and store handles.

	// Serve until we get "resources"
	for {
		var msg DriverMsg
		if err := proc.dec.Decode(&msg); err != nil {
			// driver died
			return nil, fmt.Errorf("driver error: %v", err)
		}
		if msg.Cmd == "range" {
			// For root driver, we can return empty data or handle specific logic
			data := []byte{}
			resp := RangeResp{
				Data: base64.StdEncoding.EncodeToString(data),
				Size: int64(len(data)),
			}
			if err := proc.enc.Encode(resp); err != nil {
				return nil, err
			}
			continue
		}
		if msg.Cmd == "" {
			rm := ResourcesMsg{Resources: msg.Resources}
			// create handles for each resource
			for _, r := range rm.Resources {
				h := ChildHandle{
					Name:       r.Name,
					ChildName:  resourceKey(r),
					Size:       r.Size,
					Kind:       r.Kind,
					DriverName: "root_driver",
				}
				hid := s.addHandle(h)
				resources = append(resources, Resource{
					Name:     r.Name,
					Kind:     r.Kind,
					Size:     r.Size,
					Handle:   hid,
					Readable: r.Readable,
				})
			}
			s.mu.Lock()
			s.rootResources = resources
			s.mu.Unlock()
			return resources, nil
		}
	}
}

// Load driver for a handle and return resource list
func (s *Store) loadDriverForHandle(driverName string, handle ChildHandle) (resources []Resource, err error) {
	handle, err = s.resolveHandle(handle.Hid)
	switch {
	case err != nil:
		return nil, err
	default:
	}
	// Use unique driver name for this instance
	driverInstanceName := fmt.Sprintf("%s_%s", driverName, handle.Hid)
	if _, ok := s.getDriver(driverInstanceName); ok {
		s.stopDriverWithBlobPolicy(driverInstanceName, false)
	}
	switch {
	case handle.Route != nil:
		s.setOrigin(driverInstanceName, routeOrigin{Route: *handle.Route, Driver: driverName})
	default:
	}

	proc, err := startDriver(driverName)
	if err != nil {
		return nil, err
	}

	s.addDriver(driverInstanceName, proc)

	// Serve until we get "resources"
	for {
		var msg DriverMsg
		if err := proc.dec.Decode(&msg); err != nil {
			return nil, fmt.Errorf("driver error: %v", err)
		}
		if msg.Cmd == "range" {
			// Forward to parent driver
			data, err2 := s.serveChildRange(handle, msg.Offset, msg.Length)
			resp := RangeResp{
				Data: base64.StdEncoding.EncodeToString(data),
				Size: int64(len(data)),
			}
			switch {
			case err2 != nil:
				resp.Error = err2.Error()
			default:
			}
			if err := proc.enc.Encode(resp); err != nil {
				return nil, err
			}
			continue
		}
		if msg.Cmd == "" {
			rm := ResourcesMsg{Resources: msg.Resources}
			// create handles for each resource
			for _, r := range rm.Resources {
				h := ChildHandle{
					Name:       r.Name,
					ChildName:  resourceKey(r),
					Size:       r.Size,
					Kind:       r.Kind,
					DriverName: driverInstanceName,
				}
				hid := s.addHandle(h)
				resources = append(resources, Resource{
					Name:     r.Name,
					Kind:     r.Kind,
					Size:     r.Size,
					Handle:   hid,
					Readable: r.Readable,
				})
			}
			return resources, nil
		}
	}
}

// Load driver for an uploaded blob and return resource list
func (s *Store) loadDriverForBlob(driverName string, blobID string) (resources []Resource, err error) {
	driverInstanceName := fmt.Sprintf("%s_%s", driverName, blobID)
	if _, ok := s.getDriver(driverInstanceName); ok {
		s.stopDriverWithBlobPolicy(driverInstanceName, false)
	}
	switch {
	case s.reserveBlobForDriver(driverInstanceName, blobID):
	default:
		return nil, fmt.Errorf("uploaded resource id %s is unknown or has expired", blobID)
	}

	proc, err := startDriver(driverName)
	if err != nil {
		s.releaseBlobReservation(driverInstanceName)
		return nil, err
	}
	s.addDriver(driverInstanceName, proc)
	loaded := false
	defer func() {
		switch {
		case loaded:
		default:
			s.stopDriverWithBlobPolicy(driverInstanceName, false)
		}
	}()

	// Serve until we get "resources"
	for {
		var msg DriverMsg
		if err := proc.dec.Decode(&msg); err != nil {
			return nil, fmt.Errorf("driver error: %v", err)
		}
		if msg.Cmd == "range" {
			data := s.serveBlobRange(blobID, msg.Offset, msg.Length)
			resp := RangeResp{
				Data: base64.StdEncoding.EncodeToString(data),
				Size: int64(len(data)),
			}
			if err := proc.enc.Encode(resp); err != nil {
				return nil, err
			}
			continue
		}
		if msg.Cmd == "" {
			rm := ResourcesMsg{Resources: msg.Resources}
			// create handles for each resource
			for _, r := range rm.Resources {
				h := ChildHandle{
					Name:       r.Name,
					ChildName:  resourceKey(r),
					Size:       r.Size,
					Kind:       r.Kind,
					DriverName: driverInstanceName,
				}
				hid := s.addHandle(h)
				resources = append(resources, Resource{
					Name:     r.Name,
					Kind:     r.Kind,
					Size:     r.Size,
					Handle:   hid,
					Readable: r.Readable,
				})
			}
			loaded = true
			return resources, nil
		}
	}
}

func driverForKind(kind string) (string, bool) {
	switch kind {
	case "directory":
		return "directory_driver", true
	case "git":
		return "git_driver", true
	case "github":
		return "github_driver", true
	case "aws":
		return "aws_driver", true
	case "hardware", "usb":
		return "system_profiler_driver", true
	case "diff":
		return "diff_driver", true
	case "archive", "zip":
		return "zip_driver", true
	case "kubernetes":
		return "kubernetes_driver", true
	case "json":
		return "json_driver", true
	case "k8s-pod":
		return "k8s_pod_driver", true
	case "k8s-workload":
		return "k8s_workload_driver", true
	case "k8s-service":
		return "k8s_service_driver", true
	case "k8s-configmap":
		return "k8s_configmap_driver", true
	case "k8s-node":
		return "k8s_node_driver", true
	case "url", "error", "kv", "raw":
		return "", false
	case "image":
		return "image_driver", true
	case "pdf":
		return "pdf_driver", true
	case "html":
		return "html_driver", true
	case "csv":
		return "csv_driver", true
	case "text", "file":
		return "text_driver", true
	default:
		return "text_driver", true
	}
}

// ---------------- HTTP API ----------------

func (s *Store) listDrivers() []string {
	return append([]string{}, uploadDriverNames...)
}

func resourceKey(resource Resource) string {
	switch resource.Key {
	case "":
		return resource.Name
	default:
		return resource.Key
	}
}

func runEmbeddedDriver(driverName string, args []string) {
	os.Args = append([]string{driverName}, args...)
	switch driverName {
	case "root_driver":
		rootdriver.Run()
	case "directory_driver":
		directorydriver.Run()
	case "git_driver":
		gitdriver.Run()
	case "github_driver":
		githubdriver.Run()
	case "aws_driver":
		awsdriver.Run()
	case "system_profiler_driver":
		systemprofilerdriver.Run()
	case "diff_driver":
		diffdriver.Run()
	case "zip_driver":
		zipdriver.Run()
	case "text_driver":
		textdriver.Run()
	case "kubernetes_driver":
		kubernetesdriver.Run()
	case "json_driver":
		jsondriver.Run()
	case "k8s_pod_driver":
		k8spoddriver.Run()
	case "k8s_workload_driver":
		k8sworkloaddriver.Run()
	case "k8s_service_driver":
		k8sservicedriver.Run()
	case "k8s_configmap_driver":
		k8sconfigmapdriver.Run()
	case "k8s_node_driver":
		k8snodedriver.Run()
	case "image_driver":
		imagedriver.Run()
	case "pdf_driver":
		pdfdriver.Run()
	case "html_driver":
		htmldriver.Run()
	case "csv_driver":
		csvdriver.Run()
	default:
		logUnknownEmbeddedDriver(driverName)
	}
}

func main() {
	switch {
	case len(os.Args) == 2 && os.Args[1] == "-test-recovery-worker":
		switch err := runRecoveryWorker(); {
		case err != nil:
			log.Fatal(err)
		default:
			return
		}
	case len(os.Args) > 2 && os.Args[1] == "--driver":
		runEmbeddedDriver(os.Args[2], os.Args[3:])
		return
	case len(os.Args) > 1 && os.Args[1] == "--driver":
		logMissingEmbeddedDriver()
		return
	case len(os.Args) > 1 && os.Args[1] == "-test":
		switch err := runHostSelfTest(); {
		case err != nil:
			log.Fatalf("host self-test failed: %v", err)
		default:
			log.Println("host self-test passed")
		}
		return
	case len(os.Args) > 1 && os.Args[1] == "-test-installed-layout":
		switch err := runInstalledLayoutSelfTest(); {
		case err != nil:
			logInstalledLayoutSelfTestFailure(err)
		default:
			logInstalledLayoutSelfTestPassed()
		}
		return
	default:
	}

	rootDirectory, err := resolveRootDirectory(os.Args[1:])
	switch {
	case err != nil:
		logRootDirectoryFailure(err)
	default:
	}
	store, recoveryErr := openPersistentStore()
	switch {
	case recoveryErr != nil:
		log.Fatal(recoveryErr)
	default:
	}
	store.rootDirectory = rootDirectory
	logRootDirectory(rootDirectory)

	// Auto-load root driver on startup
	go func() {
		// Give the server a moment to start up
		time.Sleep(200 * time.Millisecond)

		// Load the root driver to browse the selected directory
		resources, err := store.loadRootDriver()
		if err != nil {
			log.Printf("Failed to auto-load root driver: %v", err)
		} else {
			log.Printf("Auto-loaded root driver with resources: %d", len(resources))
		}
	}()

	// Idle-session cleanup: periodically stop driver sessions that have not
	// served a request for the configured timeout, so long-running processes
	// (and their in-memory payloads) do not pile up forever.
	idleTimeout := resolveIdleTimeout()
	switch {
	case idleTimeout > 0:
		log.Printf("idle-session cleanup enabled: sessions idle for %s will be stopped", idleTimeout)
		go func() {
			ticker := time.NewTicker(idleSweepInterval)
			defer ticker.Stop()
			for range ticker.C {
				reaped := store.reapIdleDrivers(idleTimeout)
				switch {
				case len(reaped) > 0:
					log.Printf("idle-session cleanup stopped %d session(s) idle for over %s: %s", len(reaped), idleTimeout, strings.Join(reaped, ", "))
				default:
				}
			}
		}()
	default:
		log.Printf("idle-session cleanup disabled by DATATREE_IDLE_TIMEOUT")
	}
	switch {
	case store.uploadIdleTimeout > 0:
		go func() {
			ticker := time.NewTicker(idleSweepInterval)
			defer ticker.Stop()
			for now := range ticker.C {
				store.reapIdleBlobs(now)
			}
		}()
	default:
	}

	mux := http.NewServeMux()
	mountFoldControl(mux)

	// static
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(webIndex)
	})

	// drivers list
	mux.HandleFunc("/api/drivers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(store.listDrivers())
	})

	mux.HandleFunc("/api/upload-settings", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(UploadSettingsResponse{
			MaxUploadBytes:         store.maxUploadBytes,
			MaxRetainedUploadBytes: store.maxRetainedUploadBytes,
			UploadIdleTimeout:      store.uploadIdleTimeout.String(),
			Explanation:            "Each upload must fit the per-file limit. Datatree evicts the least recently used inactive uploads to stay within the retained-memory limit; unloading a preview releases its bytes and handles immediately.",
		})
	})

	// upload raw bytes as a root blob
	mux.HandleFunc("/api/upload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required for /api/upload", http.StatusMethodNotAllowed)
			return
		}
		switch {
		case r.ContentLength > store.maxUploadBytes:
			http.Error(w, fmt.Sprintf("upload is %d bytes, but DATATREE_MAX_UPLOAD_BYTES allows at most %d bytes", r.ContentLength, store.maxUploadBytes), http.StatusRequestEntityTooLarge)
			return
		default:
		}
		data, err := io.ReadAll(io.LimitReader(r.Body, store.maxUploadBytes+1))
		if err != nil {
			http.Error(w, fmt.Sprintf("cannot read upload body: expected raw bytes, got read error: %v", err), 400)
			return
		}
		switch {
		case int64(len(data)) > store.maxUploadBytes:
			http.Error(w, fmt.Sprintf("upload exceeds DATATREE_MAX_UPLOAD_BYTES: read more than the allowed %d bytes", store.maxUploadBytes), http.StatusRequestEntityTooLarge)
			return
		default:
		}
		id, putErr := store.putBlob(data)
		if putErr != nil {
			http.Error(w, fmt.Sprintf("cannot retain upload: %v", putErr), http.StatusInsufficientStorage)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(UploadResponse{ResID: id, Size: len(data)})
	})

	// load a driver on an uploaded blob
	mux.HandleFunc("/api/load", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required for /api/load", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			ResID  string `json:"res_id"`
			Driver string `json:"driver"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("cannot decode /api/load request: expected JSON with res_id and driver, got error: %v", err), 400)
			return
		}
		if _, ok := store.getBlob(req.ResID); !ok {
			http.Error(w, fmt.Sprintf("cannot load driver %s: uploaded resource id %s is unknown", req.Driver, req.ResID), 404)
			return
		}
		res, err := store.loadDriverForBlob(req.Driver, req.ResID)
		if err != nil {
			http.Error(w, fmt.Sprintf("cannot load driver %s on uploaded resource %s: %v", req.Driver, req.ResID, err), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(LoadResponse{Resources: res, Driver: req.Driver, Session: fmt.Sprintf("%s_%s", req.Driver, req.ResID)})
	})

	// list root resources
	mux.HandleFunc("/api/list", func(w http.ResponseWriter, r *http.Request) {
		store.mu.RLock()
		resources := store.rootResources
		store.mu.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ListResponse{Resources: resources})
	})

	// auto-load appropriate driver for a handle
	mux.HandleFunc("/api/autoLoad", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Handle string `json:"handle"`
			Kind   string `json:"kind"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		h, recoveryErr := store.resolveHandle(req.Handle)
		if recoveryErr != nil {
			http.Error(w, recoveryErr.Error(), 404)
			return
		}

		// Get the appropriate driver for this kind
		driver, expandable := driverForKind(req.Kind)
		if !expandable {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(LoadResponse{Resources: []Resource{}, Driver: "leaf", Session: ""})
			return
		}

		res, err := store.loadDriverForHandle(driver, h)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(LoadResponse{Resources: res, Driver: driver, Session: fmt.Sprintf("%s_%s", driver, h.Hid)})
	})

	// load a selected driver on a child handle
	mux.HandleFunc("/api/loadChild", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required for /api/loadChild", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Handle string `json:"handle"`
			Kind   string `json:"kind"`
			Driver string `json:"driver"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("cannot decode /api/loadChild request: expected JSON with handle and optional driver, got error: %v", err), 400)
			return
		}
		h, recoveryErr := store.resolveHandle(req.Handle)
		if recoveryErr != nil {
			http.Error(w, recoveryErr.Error(), 404)
			return
		}
		driver := req.Driver
		if driver == "" {
			var expandable bool
			kind := req.Kind
			if kind == "" {
				kind = h.Kind
			}
			driver, expandable = driverForKind(kind)
			if !expandable {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(LoadResponse{Resources: []Resource{}, Driver: "leaf", Session: ""})
				return
			}
		}
		res, err := store.loadDriverForHandle(driver, h)
		if err != nil {
			http.Error(w, fmt.Sprintf("cannot load driver %s on handle %s child %s: %v", driver, h.Hid, h.ChildName, err), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(LoadResponse{Resources: res, Driver: driver, Session: fmt.Sprintf("%s_%s", driver, h.Hid)})
	})

	// unload a running driver session
	mux.HandleFunc("/api/unload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required for /api/unload", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Session string `json:"session"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("cannot decode /api/unload request: expected JSON with session, got error: %v", err), 400)
			return
		}
		stopped := store.stopDriver(req.Session)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(UnloadResponse{Session: req.Session, Stopped: stopped})
	})

	mux.HandleFunc("/api/github/action", githubActionHandler(store))
	mux.HandleFunc("/api/aws/action", awsActionHandler(store))

	// data (download bytes via handle)
	mux.HandleFunc("/api/data", func(w http.ResponseWriter, r *http.Request) {
		hid := r.URL.Query().Get("handle")
		off := parseI64(r.URL.Query().Get("offset"), 0)
		length := parseI64(r.URL.Query().Get("length"), -1)

		h, recoveryErr := store.resolveHandle(hid)
		if recoveryErr != nil {
			http.Error(w, recoveryErr.Error(), 404)
			return
		}
		chunk, err := store.serveChildRange(h, off, length)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.Itoa(len(chunk)))
		_, _ = w.Write(chunk)
	})

	addr := resolveListenAddr()
	listener, err := listenPersistent(addr)
	switch {
	case err != nil:
		logListenFailure(addr, err)
	default:
	}
	url := browserURL(listener.Addr().String())
	galleryAssets := os.Getenv("DATATREE_GALLERY_ASSETS")
	switch {
	case galleryAssets == "":
		// The regular launcher opens the desktop browser.
	case !listener.Addr().(*net.TCPAddr).IP.IsLoopback() || rootDirectory == "":
		log.Fatal("Datatree gallery requires a loopback listener and an explicit directory to browse")
	default:
		mountGallery(mux, galleryAssets)
		url += galleryPath
	}
	logServingURL(url)
	switch err := exec.Command("/usr/bin/open", url).Start(); {
	case err != nil:
		logBrowserOpenFailure(url, err)
	default:
	}
	logHTTPServerFailure(url, http.Serve(listener, browserHandler(mux, listener.Addr().(*net.TCPAddr))))
}

const maxGitHubActionRequestBytes int64 = 70 * 1024
const maxAWSActionRequestBytes int64 = 270 * 1024

func awsActionHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method != http.MethodPost:
			http.Error(w, "POST required for /api/aws/action", http.StatusMethodNotAllowed)
			return
		case r.Header.Get("Origin") != "http://"+r.Host:
			http.Error(w, fmt.Sprintf("AWS action refused: expected same-origin request from %q, got Origin %q", "http://"+r.Host, r.Header.Get("Origin")), http.StatusForbidden)
			return
		default:
		}
		var req AWSActionRequest
		decoder := json.NewDecoder(io.LimitReader(r.Body, maxAWSActionRequestBytes+1))
		switch err := decoder.Decode(&req); {
		case err != nil:
			http.Error(w, fmt.Sprintf("cannot decode AWS action request: expected JSON with handle and optional body within %d bytes, got: %v", maxAWSActionRequestBytes, err), http.StatusBadRequest)
			return
		default:
		}
		message, err := executeAWSAction(store, req)
		switch {
		case err != nil:
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(AWSActionResponse{Message: message})
	}
}

func executeAWSAction(store *Store, req AWSActionRequest) (string, error) {
	handle, recoveryErr := store.resolveHandle(req.Handle)
	switch {
	case recoveryErr != nil:
		return "", fmt.Errorf("cannot execute AWS action: handle %q is unknown or unavailable: %v", req.Handle, recoveryErr)
	case !strings.HasPrefix(handle.Kind, "aws-action-"):
		return "", fmt.Errorf("cannot execute AWS action for handle %q: expected kind aws-action-*, got %q", req.Handle, handle.Kind)
	default:
	}
	target, err := store.serveChildRange(handle, 0, -1)
	switch {
	case err != nil:
		return "", fmt.Errorf("cannot execute AWS action %q for handle %q: failed to read its validated target: %v", handle.Kind, req.Handle, err)
	default:
	}
	result, actionErr := awsdriver.ExecuteAction(handle.Kind, target, req.Body)
	switch {
	case actionErr != nil:
		return "", actionErr
	default:
		return string(result), nil
	}
}

func githubActionHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method != http.MethodPost:
			http.Error(w, "POST required for /api/github/action", http.StatusMethodNotAllowed)
			return
		case r.Header.Get("Origin") != "http://"+r.Host:
			http.Error(w, fmt.Sprintf("GitHub action refused: expected same-origin request from %q, got Origin %q", "http://"+r.Host, r.Header.Get("Origin")), http.StatusForbidden)
			return
		default:
		}
		var req GitHubActionRequest
		decoder := json.NewDecoder(io.LimitReader(r.Body, maxGitHubActionRequestBytes+1))
		switch err := decoder.Decode(&req); {
		case err != nil:
			http.Error(w, fmt.Sprintf("cannot decode GitHub action request: expected JSON with handle and optional body or merge_method within %d bytes, got: %v", maxGitHubActionRequestBytes, err), http.StatusBadRequest)
			return
		default:
		}
		message, err := executeGitHubAction(store, req)
		switch {
		case err != nil:
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		default:
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(GitHubActionResponse{Message: message})
	}
}

func executeGitHubAction(store *Store, req GitHubActionRequest) (string, error) {
	handle, recoveryErr := store.resolveHandle(req.Handle)
	switch {
	case recoveryErr != nil:
		return "", fmt.Errorf("cannot execute GitHub action: handle %q is unknown or unavailable: %v", req.Handle, recoveryErr)
	case !strings.HasPrefix(handle.Kind, "github-action-"):
		return "", fmt.Errorf("cannot execute GitHub action for handle %q: expected kind github-action-*, got %q", req.Handle, handle.Kind)
	default:
	}
	target, err := store.serveChildRange(handle, 0, -1)
	switch {
	case err != nil:
		return "", fmt.Errorf("cannot execute GitHub action %q for handle %q: failed to read its validated target: %v", handle.Kind, req.Handle, err)
	default:
	}
	result, actionErr := githubdriver.ExecuteAction(handle.Kind, target, req.Body, req.MergeMethod)
	switch {
	case actionErr != nil:
		return "", actionErr
	default:
		return string(result), nil
	}
}

func localBrowserAuthority(authority string, listen *net.TCPAddr) bool {
	switch {
	case !listen.IP.IsLoopback():
		return true
	default:
		host, port, err := net.SplitHostPort(authority)
		switch {
		case err != nil:
			return listen.Port == 80 && (strings.EqualFold(authority, "localhost") ||
				authority == strings.TrimSuffix(net.JoinHostPort(listen.IP.String(), "80"), ":80"))
		default:
			return port == strconv.Itoa(listen.Port) &&
				(strings.EqualFold(host, "localhost") || listen.IP.Equal(net.ParseIP(host)))
		}
	}
}

func browserHandler(h http.Handler, listen *net.TCPAddr) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		switch {
		case !localBrowserAuthority(r.Host, listen):
			http.Error(w, "Datatree requires the loopback host and port it is listening on", http.StatusForbidden)
			return
		case strings.HasPrefix(r.URL.Path, "/api/") &&
			((origin != "" && origin != "http://"+r.Host) || r.Header.Get("Sec-Fetch-Site") == "cross-site" || r.Header.Get("Sec-Fetch-Site") == "same-site"):
			http.Error(w, "Datatree API requires a same-origin browser request", http.StatusForbidden)
			return
		default:
			// Allow direct clients and navigation to the browser page.
		}
		// correct content-type for .html without extension madness
		if strings.HasSuffix(r.URL.Path, ".html") {
			w.Header().Set("Content-Type", mime.TypeByExtension(".html"))
		}
		h.ServeHTTP(w, r)
	})
}

func parseI64(s string, def int64) int64 {
	if s == "" {
		return def
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return def
	}
	return n
}
