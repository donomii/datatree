package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// fakePodManifest is the JSON manifest the fake kubectl serves for the test
// pod. It must stay in sync with fakeKubectlScript. It is JSON because the
// kubernetes driver fetches objects with -o json; the spec and status fields
// exercise the k8s_pod_driver curated view, and the nested metadata,
// annotations, and containers exercise the json_driver walk at depth.
const fakePodManifest = "{\"apiVersion\":\"v1\",\"kind\":\"Pod\",\"metadata\":{\"name\":\"datatree-pod\",\"annotations\":{\"docs\":\"https://example.com/pod-docs\"}},\"spec\":{\"nodeName\":\"datatree-node\",\"containers\":[{\"image\":\"datatree/pod:latest\",\"name\":\"main\"}]},\"status\":{\"phase\":\"Running\",\"podIP\":\"10.244.0.7\",\"containerStatuses\":[{\"name\":\"main\",\"ready\":true,\"restartCount\":2}]}}"

const fakeSystemProfilerHardwareJSON = `{"SPHardwareDataType":[{"_name":"hardware_overview","chip_type":"Test Chip","machine_name":"Datatree Test Mac"}]}`
const fakeSystemProfilerUSBJSON = `{"SPUSBHostDataType":[{"_items":[{"_name":"Test Keyboard","USBDeviceKeyProductID":"0x1234"}],"_name":"Test USB Bus","Driver":"TestUSB"}]}`

func fakeSystemProfilerScript() string {
	return "#!/bin/sh\n" +
		"case \"$*\" in\n" +
		"'-listDataTypes') printf 'Available Datatypes:\\nSPHardwareDataType\\nSPUSBHostDataType\\n' ;;\n" +
		"'SPHardwareDataType -json') printf '%s' '" + fakeSystemProfilerHardwareJSON + "' ;;\n" +
		"'SPUSBHostDataType -json') printf '%s' '" + fakeSystemProfilerUSBJSON + "' ;;\n" +
		"*) printf 'fake system_profiler refusing unsupported invocation: %s\\n' \"$*\" >&2; exit 7 ;;\n" +
		"esac\n"
}

// fakeKubectlScript builds a stand-in kubectl so the self-test never contacts
// a real cluster. It logs every invocation to logPath for the read-only audit
// and answers the exact read-only invocations the kubernetes driver makes.
func fakeKubectlScript(logPath string, kubeconfigPath string) string {
	return "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> '" + logPath + "'\n" +
		"if [ \"$KUBECONFIG\" != '" + kubeconfigPath + "' ]; then echo \"expected KUBECONFIG=" + kubeconfigPath + ", got $KUBECONFIG\" >&2; exit 4; fi\n" +
		"case \"$*\" in\n" +
		"'config get-contexts -o name') printf 'second-test-context\\ndatatree-test-context\\ndatatree-test-context\\n' ;;\n" +
		"'get --context=datatree-test-context namespaces -o name') printf 'namespace/datatree-ns\\nnamespace/kube-system\\n' ;;\n" +
		"'api-resources --context=datatree-test-context --verbs=list --namespaced=true -o name') printf 'widgets.example.test\\nservices\\nsecrets\\nreplicasets.apps\\npods\\njobs.batch\\ndeployments.apps\\ndaemonsets.apps\\ncronjobs.batch\\nconfigmaps\\nstatefulsets.apps\\n' ;;\n" +
		"'api-resources --context=datatree-test-context --verbs=list --namespaced=false -o name') printf 'nodes\\n' ;;\n" +
		"'get --context=datatree-test-context --namespace=datatree-ns pods -o name') printf 'pod/datatree-pod\\n' ;;\n" +
		"'get --context=datatree-test-context --namespace=datatree-ns pods datatree-pod -o json') printf '%s' '" + fakePodManifest + "' ;;\n" +
		"'get --context=datatree-test-context --namespace=datatree-ns deployments.apps -o name') printf 'deployment/datatree-deployment\\n' ;;\n" +
		"'get --context=datatree-test-context --namespace=datatree-ns statefulsets.apps -o name') printf 'statefulset/datatree-statefulset\\n' ;;\n" +
		"'get --context=datatree-test-context --namespace=datatree-ns daemonsets.apps -o name') printf 'daemonset/datatree-daemonset\\n' ;;\n" +
		"'get --context=datatree-test-context --namespace=datatree-ns replicasets.apps -o name') printf 'replicaset/datatree-replicaset\\n' ;;\n" +
		"'get --context=datatree-test-context --namespace=datatree-ns jobs.batch -o name') printf 'job/datatree-job\\n' ;;\n" +
		"'get --context=datatree-test-context --namespace=datatree-ns cronjobs.batch -o name') printf 'cronjob/datatree-cronjob\\n' ;;\n" +
		"'get --context=datatree-test-context --namespace=datatree-ns services -o name') printf 'service/datatree-service\\n' ;;\n" +
		"'get --context=datatree-test-context --namespace=datatree-ns configmaps -o name') printf 'configmap/datatree-config\\n' ;;\n" +
		"'get --context=datatree-test-context --namespace=datatree-ns secrets -o name') printf 'secret/blocked-secret\\n' ;;\n" +
		"'get --context=datatree-test-context --namespace=datatree-ns secrets blocked-secret -o json') echo 'access denied by the test credential' >&2; exit 5 ;;\n" +
		"'get --context=datatree-test-context --namespace=datatree-ns widgets.example.test -o name') printf 'widget/datatree-widget\\n' ;;\n" +
		"'get --context=datatree-test-context nodes -o name') printf 'node/datatree-node\\n' ;;\n" +
		"*) echo \"fake kubectl refusing unsupported invocation: $*\" >&2; exit 3 ;;\n" +
		"esac\n"
}

// auditKubectlCallLog verifies the read-only guarantee: every kubectl
// invocation recorded by the fake kubectl must be get, api-resources, or
// config get-contexts.
func auditKubectlCallLog(logPath string) error {
	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		return fmt.Errorf("cannot read the fake kubectl call log %s: %v", logPath, err)
	}
	calls := 0
	for _, line := range strings.Split(string(logBytes), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		calls++
		fields := strings.Fields(trimmed)
		switch {
		case fields[0] == "get":
		case fields[0] == "api-resources":
		case fields[0] == "config" && len(fields) > 1 && fields[1] == "get-contexts":
		default:
			return fmt.Errorf("read-only violation: the kubernetes driver ran kubectl %q, but only get, api-resources and config get-contexts are allowed", trimmed)
		}
	}
	if calls == 0 {
		return fmt.Errorf("read-only audit found no kubectl calls in %s: expected the kubernetes walk to invoke the fake kubectl", logPath)
	}
	return nil
}

type testZipEntry struct {
	Name string
	Data []byte
}

func makeTestZip(entries []testZipEntry) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, entry := range entries {
		w, err := zw.Create(entry.Name)
		if err != nil {
			return nil, err
		}
		_, err = w.Write(entry.Data)
		if err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func findResource(resources []Resource, name string, kind string) (Resource, error) {
	for _, resource := range resources {
		if resource.Name == name && (kind == "" || resource.Kind == kind) {
			return resource, nil
		}
	}
	return Resource{}, fmt.Errorf("resource not found: expected name %s kind %s in %d resources", name, kind, len(resources))
}

func findResourceHandle(store *Store, resources []Resource, name string, kind string) (ChildHandle, error) {
	resource, err := findResource(resources, name, kind)
	if err != nil {
		return ChildHandle{}, err
	}
	if resource.Handle == "" {
		return ChildHandle{}, fmt.Errorf("resource %s kind %s did not include a handle", name, kind)
	}
	handle, ok := store.getHandle(resource.Handle)
	if !ok {
		return ChildHandle{}, fmt.Errorf("resource %s kind %s referenced unknown handle %s", name, kind, resource.Handle)
	}
	return handle, nil
}

func runInstalledLayoutSelfTest() error {
	executablePath, executableErr := os.Executable()
	switch {
	case executableErr != nil:
		return fmt.Errorf("installed executable path unavailable: expected one Datatree binary, got error: %v", executableErr)
	default:
	}
	executableInfo, executableErr := os.Stat(executablePath)
	switch {
	case executableErr != nil:
		return fmt.Errorf("installed executable unreadable: expected %s, got stat error: %v", executablePath, executableErr)
	case executableInfo.Mode()&0111 == 0:
		return fmt.Errorf("installed executable is not executable: expected execute permission on %s, got mode %s", executablePath, executableInfo.Mode())
	case !bytes.Contains(webIndex, []byte("Directory Browser")):
		return fmt.Errorf("embedded web UI mismatch: expected the Datatree binary %s to contain the Directory Browser heading", executablePath)
	default:
	}
	store := NewStore()
	defer store.stopDrivers()
	rootResources, rootErr := store.loadRootDriver()
	switch {
	case rootErr != nil:
		return fmt.Errorf("installed embedded root driver failed from %s: %v", executablePath, rootErr)
	case rootResources == nil:
		return fmt.Errorf("installed embedded root driver returned a nil resource list from %s, expected a JSON resources response", executablePath)
	default:
	}
	blobID, blobErr := store.putBlob([]byte("{\"installed\":true}"))
	switch {
	case blobErr != nil:
		return fmt.Errorf("cannot retain installed-layout JSON fixture: %v", blobErr)
	default:
	}
	jsonResources, jsonErr := store.loadDriverForBlob("json_driver", blobID)
	switch {
	case jsonErr != nil:
		return fmt.Errorf("installed embedded JSON driver failed from %s: %v", executablePath, jsonErr)
	default:
	}
	if _, err := findResource(jsonResources, "installed: true", "kv"); err != nil {
		return err
	}
	return nil
}

func checkKubernetesObjectKind(store *Store, typeResources []Resource, typeName string, objectName string, kind string) error {
	typeHandle, err := findResourceHandle(store, typeResources, typeName, "kubernetes")
	if err != nil {
		return err
	}
	objectResources, err := store.loadDriverForHandle("kubernetes_driver", typeHandle)
	if err != nil {
		return fmt.Errorf("kubernetes driver load failed for resource type %s: %v", typeName, err)
	}
	if _, err := findResource(objectResources, objectName, kind); err != nil {
		return err
	}
	return nil
}

func checkUploadRetention() error {
	store := NewStoreWithUploadLimits(4, 5, time.Minute)
	firstID, err := store.putBlob([]byte("one"))
	if err != nil {
		return fmt.Errorf("cannot retain first bounded upload: %v", err)
	}
	store.mu.Lock()
	firstBlob := store.blobs[firstID]
	firstBlob.lastUsed = time.Now().Add(-2 * time.Minute)
	store.blobs[firstID] = firstBlob
	store.mu.Unlock()
	secondID, err := store.putBlob([]byte("two"))
	if err != nil {
		return fmt.Errorf("cannot retain second bounded upload: %v", err)
	}
	if _, ok := store.getBlob(firstID); ok {
		return fmt.Errorf("retained upload budget kept oldest upload %s after %s needed its capacity", firstID, secondID)
	}
	if !store.reserveBlobForDriver("active", secondID) {
		return fmt.Errorf("cannot reserve retained upload %s for active driver test", secondID)
	}
	if _, err := store.putBlob([]byte("four")); err == nil {
		return fmt.Errorf("retained upload budget accepted four bytes with three active bytes under a five-byte limit")
	}
	store.releaseBlobReservation("active")
	if _, err := store.putBlob([]byte("four")); err != nil {
		return fmt.Errorf("retained upload budget did not evict inactive upload %s: %v", secondID, err)
	}
	if _, err := store.putBlob([]byte("large")); err == nil {
		return fmt.Errorf("per-upload limit accepted five bytes under a four-byte limit")
	}

	expiringStore := NewStoreWithUploadLimits(4, 8, time.Minute)
	expiringID, err := expiringStore.putBlob([]byte("old"))
	if err != nil {
		return fmt.Errorf("cannot retain upload for idle-expiry test: %v", err)
	}
	expiringStore.mu.Lock()
	expiringBlob := expiringStore.blobs[expiringID]
	expiringBlob.lastUsed = time.Now().Add(-2 * time.Minute)
	expiringStore.blobs[expiringID] = expiringBlob
	expiringStore.mu.Unlock()
	reaped := expiringStore.reapIdleBlobs(time.Now())
	if len(reaped) != 1 || reaped[0] != expiringID {
		return fmt.Errorf("idle upload cleanup reaped %v, expected only %s", reaped, expiringID)
	}
	return nil
}

// checkPodManifestBrowse drills the pod's [manifest] child through
// json_driver, proving that the complete manifest stays browsable as
// hierarchical data behind the curated pod view: metadata.annotations.docs is
// reached as a leaf, and spec.containers[0].image is reached through an
// object, an array, and an object element.
func checkPodManifestBrowse(store *Store, manifestHandle ChildHandle) error {
	podFields, err := store.loadDriverForHandle("json_driver", manifestHandle)
	if err != nil {
		return fmt.Errorf("json driver load failed for the pod manifest: %v", err)
	}
	if _, err := findResource(podFields, "apiVersion: v1", "kv"); err != nil {
		return err
	}
	metadataHandle, err := findResourceHandle(store, podFields, "metadata", "json")
	if err != nil {
		return err
	}
	metadataFields, err := store.loadDriverForHandle("json_driver", metadataHandle)
	if err != nil {
		return fmt.Errorf("json driver load failed for pod metadata: %v", err)
	}
	annotationsHandle, err := findResourceHandle(store, metadataFields, "annotations", "json")
	if err != nil {
		return err
	}
	annotationFields, err := store.loadDriverForHandle("json_driver", annotationsHandle)
	if err != nil {
		return fmt.Errorf("json driver load failed for pod annotations: %v", err)
	}
	if _, err := findResource(annotationFields, "docs: https://example.com/pod-docs", "kv"); err != nil {
		return err
	}
	specHandle, err := findResourceHandle(store, podFields, "spec", "json")
	if err != nil {
		return err
	}
	specFields, err := store.loadDriverForHandle("json_driver", specHandle)
	if err != nil {
		return fmt.Errorf("json driver load failed for pod spec: %v", err)
	}
	containersHandle, err := findResourceHandle(store, specFields, "containers", "json")
	if err != nil {
		return err
	}
	containerElements, err := store.loadDriverForHandle("json_driver", containersHandle)
	if err != nil {
		return fmt.Errorf("json driver load failed for pod spec containers: %v", err)
	}
	firstContainerHandle, err := findResourceHandle(store, containerElements, "[0]", "json")
	if err != nil {
		return err
	}
	containerFields, err := store.loadDriverForHandle("json_driver", firstContainerHandle)
	if err != nil {
		return fmt.Errorf("json driver load failed for the first pod container: %v", err)
	}
	if _, err := findResource(containerFields, "image: datatree/pod:latest", "kv"); err != nil {
		return err
	}
	return nil
}

func checkRootDirectorySelection() error {
	resolvedDefaultDirectory, err := resolveRootDirectory(nil)
	switch {
	case err != nil:
		return fmt.Errorf("cannot select the default location browser: %v", err)
	case resolvedDefaultDirectory != "":
		return fmt.Errorf("default-root resolution mismatch: expected location-browser selection, got directory %q", resolvedDefaultDirectory)
	default:
	}
	locationStore := NewStore()
	defer locationStore.stopDrivers()
	locationResources, err := locationStore.loadRootDriver()
	if err != nil {
		return fmt.Errorf("cannot load the default location browser: %v", err)
	} else {
		homeHandle, err := findResourceHandle(locationStore, locationResources, "HOME", "directory")
		if err != nil {
			return fmt.Errorf("default location browser did not expose HOME: %v", err)
		} else {
		}
		if _, err := locationStore.loadDriverForHandle("directory_driver", homeHandle); err != nil {
			return fmt.Errorf("default location browser HOME entry did not expand through directory_driver: %v", err)
		} else {
		}
		rootHandle, err := findResourceHandle(locationStore, locationResources, "/", "directory")
		if err != nil {
			return fmt.Errorf("default location browser did not expose /: %v", err)
		} else {
		}
		if _, err := locationStore.loadDriverForHandle("directory_driver", rootHandle); err != nil {
			return fmt.Errorf("default location browser / entry did not expand through directory_driver: %v", err)
		} else {
		}
	}
	rootDirectory, err := os.MkdirTemp("", "datatree-selected-root-")
	if err != nil {
		return fmt.Errorf("cannot create selected-root test directory: %v", err)
	}
	defer os.RemoveAll(rootDirectory)
	selectedFile := filepath.Join(rootDirectory, "selected.txt")
	if err := os.WriteFile(selectedFile, []byte("selected root"), 0600); err != nil {
		return fmt.Errorf("cannot write selected-root file %s: %v", selectedFile, err)
	}
	shadowResources := []struct {
		name string
		kind string
	}{
		{name: "hardware", kind: "hardware"},
		{name: "usb", kind: "usb"},
		{name: "kubernetes", kind: "kubernetes"},
		{name: "github", kind: "github"},
		{name: "aws", kind: "aws"},
	}
	for _, shadowResource := range shadowResources {
		shadowPath := filepath.Join(rootDirectory, shadowResource.name)
		switch err := os.WriteFile(shadowPath, []byte("selected "+shadowResource.name), 0600); {
		case err != nil:
			return fmt.Errorf("cannot write selected-root collision file %s: %v", shadowPath, err)
		default:
		}
	}
	resolvedDirectory, err := resolveRootDirectory([]string{rootDirectory})
	if err != nil {
		return fmt.Errorf("cannot resolve selected-root argument %s: %v", rootDirectory, err)
	}
	if resolvedDirectory != rootDirectory {
		return fmt.Errorf("selected-root resolution mismatch: expected %s, got %s", rootDirectory, resolvedDirectory)
	}
	store := NewStore()
	store.rootDirectory = resolvedDirectory
	defer store.stopDrivers()
	resources, err := store.loadRootDriver()
	if err != nil {
		return fmt.Errorf("cannot load selected root directory %s: %v", resolvedDirectory, err)
	}
	if _, err := findResource(resources, "selected.txt", "text"); err != nil {
		return fmt.Errorf("selected root directory %s did not expose selected.txt: %v", resolvedDirectory, err)
	}
	for _, shadowResource := range shadowResources {
		shadowHandle, handleErr := findResourceHandle(store, resources, shadowResource.name, "file")
		switch {
		case handleErr != nil:
			return fmt.Errorf("selected root directory %s did not preserve the real %s file: %v", resolvedDirectory, shadowResource.name, handleErr)
		default:
		}
		shadowData, rangeErr := store.serveChildRange(shadowHandle, 0, -1)
		switch {
		case rangeErr != nil:
			return fmt.Errorf("selected root directory %s could not read the real %s file: %v", resolvedDirectory, shadowResource.name, rangeErr)
		case string(shadowData) != "selected "+shadowResource.name:
			return fmt.Errorf("selected root directory %s returned %q for the real %s file, expected %q", resolvedDirectory, string(shadowData), shadowResource.name, "selected "+shadowResource.name)
		default:
		}
		virtualHandle, virtualErr := findResourceHandle(store, resources, shadowResource.name, shadowResource.kind)
		switch {
		case virtualErr != nil:
			return fmt.Errorf("selected root directory %s did not retain virtual %s alongside the real file: %v", resolvedDirectory, shadowResource.name, virtualErr)
		case virtualHandle.ChildName == shadowHandle.ChildName:
			return fmt.Errorf("selected root directory %s routed real and virtual %s through the same child key %q", resolvedDirectory, shadowResource.name, virtualHandle.ChildName)
		default:
		}
	}
	if _, err := resolveRootDirectory([]string{rootDirectory, rootDirectory}); err == nil {
		return fmt.Errorf("directory argument validation accepted two directories, expected exactly zero or one")
	}
	return nil
}

func checkLocalMachineBrowse(store *Store, rootResources []Resource) error {
	switch err := checkLocalHardwareBrowse(store, rootResources); {
	case err != nil:
		return err
	default:
	}
	return checkLocalUSBBrowse(store, rootResources)
}

func checkLocalHardwareBrowse(store *Store, rootResources []Resource) error {
	hardwareHandle, err := findResourceHandle(store, rootResources, "hardware", "hardware")
	switch {
	case err != nil:
		return err
	default:
	}
	hardwareResources, err := store.loadDriverForHandle("system_profiler_driver", hardwareHandle)
	switch {
	case err != nil:
		return fmt.Errorf("system profiler driver could not expand local hardware: %v", err)
	default:
	}
	switch _, err := findResource(hardwareResources, "model name: Datatree Test Mac", "kv"); {
	case err != nil:
		return err
	default:
		return nil
	}
}

func checkLocalUSBBrowse(store *Store, rootResources []Resource) error {
	usbHandle, err := findResourceHandle(store, rootResources, "usb", "usb")
	switch {
	case err != nil:
		return err
	default:
	}
	busResources, err := store.loadDriverForHandle("system_profiler_driver", usbHandle)
	switch {
	case err != nil:
		return fmt.Errorf("system profiler driver could not expand local USB buses: %v", err)
	default:
	}
	busHandle, err := findResourceHandle(store, busResources, "Test USB Bus", "usb")
	switch {
	case err != nil:
		return err
	default:
	}
	deviceResources, err := store.loadDriverForHandle("system_profiler_driver", busHandle)
	switch {
	case err != nil:
		return fmt.Errorf("system profiler driver could not expand local USB bus: %v", err)
	default:
	}
	deviceHandle, err := findResourceHandle(store, deviceResources, "Test Keyboard", "usb")
	switch {
	case err != nil:
		return err
	default:
	}
	detailResources, err := store.loadDriverForHandle("system_profiler_driver", deviceHandle)
	switch {
	case err != nil:
		return fmt.Errorf("system profiler driver could not expand local USB device: %v", err)
	default:
	}
	_, err = findResource(detailResources, "product ID: 0x1234", "kv")
	return err
}

func checkGitRootEntry(repository string) error {
	store := NewStore()
	store.rootDirectory = repository
	defer store.stopDrivers()
	resources, err := store.loadRootDriver()
	switch {
	case err != nil:
		return fmt.Errorf("root driver could not list Git repository root %s: %v", repository, err)
	default:
	}
	repositoryHandle, err := findResourceHandle(store, resources, ".", "git")
	switch {
	case err != nil:
		return fmt.Errorf("Git repository root %s was not exposed as the current repository row: %v", repository, err)
	default:
	}
	repositoryResources, err := store.loadDriverForHandle("git_driver", repositoryHandle)
	switch {
	case err != nil:
		return fmt.Errorf("Git driver could not expand the current repository row for %s: %v", repository, err)
	default:
	}
	switch _, err := findResource(repositoryResources, "branch: main", "kv"); {
	case err != nil:
		return err
	default:
		return nil
	}
}

func runHostSelfTest() error {
	if err := checkUploadRetention(); err != nil {
		return err
	}
	store := NewStore()
	defer store.stopDrivers()

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot locate home directory for root driver test: %v", err)
	}
	fixtureName := fmt.Sprintf("datatree-test-%d", time.Now().UnixNano())
	fixtureDir := filepath.Join(homeDir, fixtureName)
	if err := os.Mkdir(fixtureDir, 0700); err != nil {
		return fmt.Errorf("cannot create fixture directory %s: %v", fixtureDir, err)
	}
	defer os.RemoveAll(fixtureDir)

	repositoryPath, err := prepareGitFixture(fixtureDir)
	switch {
	case err != nil:
		return err
	default:
	}
	switch err := checkGitRootEntry(repositoryPath); {
	case err != nil:
		return err
	default:
	}
	noteData := []byte("visit https://example.com/path for datatree\n")
	if err := os.WriteFile(filepath.Join(fixtureDir, "note.txt"), noteData, 0600); err != nil {
		return fmt.Errorf("cannot write fixture text file in %s: %v", fixtureDir, err)
	}
	nestedZip, err := makeTestZip([]testZipEntry{
		{Name: "deep.txt", Data: []byte("deep https://example.com/deep\n")},
	})
	if err != nil {
		return fmt.Errorf("cannot create nested zip fixture: %v", err)
	}
	archiveZip, err := makeTestZip([]testZipEntry{
		{Name: "inner.txt", Data: []byte("inner https://example.com/inner\n")},
		{Name: "nested.zip", Data: nestedZip},
	})
	if err != nil {
		return fmt.Errorf("cannot create top-level zip fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fixtureDir, "archive.zip"), archiveZip, 0600); err != nil {
		return fmt.Errorf("cannot write fixture zip file in %s: %v", fixtureDir, err)
	}

	fakeKubectlPath := filepath.Join(fixtureDir, "fake_kubectl")
	kubectlLogPath := filepath.Join(fixtureDir, "kubectl_calls.log")
	fakeGitHubPath := filepath.Join(fixtureDir, "fake_gh")
	githubLogPath := filepath.Join(fixtureDir, "github_calls.log")
	fakeAWSPath := filepath.Join(fixtureDir, "fake_aws")
	fakeSystemProfilerPath := filepath.Join(fixtureDir, "fake_system_profiler")
	kubeconfigPath := filepath.Join(fixtureDir, "selected-kubeconfig")
	if err := os.WriteFile(kubeconfigPath, []byte("apiVersion: v1\nkind: Config\n"), 0600); err != nil {
		return fmt.Errorf("cannot write selected kubeconfig %s: %v", kubeconfigPath, err)
	}
	if err := os.WriteFile(fakeKubectlPath, []byte(fakeKubectlScript(kubectlLogPath, kubeconfigPath)), 0700); err != nil {
		return fmt.Errorf("cannot write fake kubectl script %s: %v", fakeKubectlPath, err)
	}
	if err := os.WriteFile(fakeGitHubPath, []byte(fakeGitHubScript(githubLogPath)), 0700); err != nil {
		return fmt.Errorf("cannot write fake GitHub CLI script %s: %v", fakeGitHubPath, err)
	}
	switch err := os.WriteFile(fakeAWSPath, []byte("#!/bin/sh\nexit 0\n"), 0700); {
	case err != nil:
		return fmt.Errorf("cannot write fake AWS CLI script %s: %v", fakeAWSPath, err)
	default:
	}
	switch err := os.WriteFile(fakeSystemProfilerPath, []byte(fakeSystemProfilerScript()), 0700); {
	case err != nil:
		return fmt.Errorf("cannot write fake system_profiler script %s: %v", fakeSystemProfilerPath, err)
	default:
	}
	if err := os.Setenv("DATATREE_KUBECTL", fakeKubectlPath); err != nil {
		return fmt.Errorf("cannot point DATATREE_KUBECTL at the fake kubectl %s: %v", fakeKubectlPath, err)
	}
	defer os.Unsetenv("DATATREE_KUBECTL")
	if err := os.Setenv("DATATREE_GH", fakeGitHubPath); err != nil {
		return fmt.Errorf("cannot point DATATREE_GH at the fake GitHub CLI %s: %v", fakeGitHubPath, err)
	}
	defer os.Unsetenv("DATATREE_GH")
	switch err := os.Setenv("DATATREE_AWS", fakeAWSPath); {
	case err != nil:
		return fmt.Errorf("cannot point DATATREE_AWS at the fake AWS CLI %s: %v", fakeAWSPath, err)
	default:
	}
	defer os.Unsetenv("DATATREE_AWS")
	switch err := os.Setenv("DATATREE_SYSTEM_PROFILER", fakeSystemProfilerPath); {
	case err != nil:
		return fmt.Errorf("cannot point DATATREE_SYSTEM_PROFILER at the fake system_profiler %s: %v", fakeSystemProfilerPath, err)
	default:
	}
	defer os.Unsetenv("DATATREE_SYSTEM_PROFILER")
	switch err := os.Setenv("DATATREE_SYSTEM_PROFILER_MAX_OUTPUT_BYTES", "8388608"); {
	case err != nil:
		return fmt.Errorf("cannot set the fake system_profiler output limit: %v", err)
	default:
	}
	defer os.Unsetenv("DATATREE_SYSTEM_PROFILER_MAX_OUTPUT_BYTES")
	switch err := os.Setenv("DATATREE_SYSTEM_PROFILER_TIMEOUT", "30s"); {
	case err != nil:
		return fmt.Errorf("cannot set the fake system_profiler timeout: %v", err)
	default:
	}
	defer os.Unsetenv("DATATREE_SYSTEM_PROFILER_TIMEOUT")
	if err := os.Setenv("DATATREE_KUBECONFIG", kubeconfigPath); err != nil {
		return fmt.Errorf("cannot select kubeconfig %s: %v", kubeconfigPath, err)
	}
	defer os.Unsetenv("DATATREE_KUBECONFIG")
	switch err := checkRootDirectorySelection(); {
	case err != nil:
		return err
	default:
	}

	store.rootDirectory = homeDir
	rootResources, err := store.loadRootDriver()
	if err != nil {
		return fmt.Errorf("root driver load failed: %v", err)
	}
	fixtureHandle, err := findResourceHandle(store, rootResources, fixtureName, "directory")
	if err != nil {
		return err
	}
	dirResources, err := store.loadDriverForHandle("directory_driver", fixtureHandle)
	if err != nil {
		return fmt.Errorf("directory driver load failed for fixture %s: %v", fixtureName, err)
	}
	if err := checkGitBrowse(store, dirResources); err != nil {
		return err
	}
	if err := checkGitHubBrowse(store, rootResources); err != nil {
		return err
	}
	switch err := checkS3Composition(store, fakeAWSPath, archiveZip); {
	case err != nil:
		return fmt.Errorf("S3 composition self-test failed: %v", err)
	default:
	}
	switch err := checkRestartRecovery(fixtureDir); {
	case err != nil:
		return fmt.Errorf("restart recovery self-test failed: %v", err)
	default:
	}
	switch err := checkAllFilterBytes(store, archiveZip); {
	case err != nil:
		return fmt.Errorf("filter byte-contract self-test failed: %v", err)
	default:
	}
	localMachineStore := NewStore()
	defer localMachineStore.stopDrivers()
	localMachineResources, err := localMachineStore.loadRootDriver()
	switch {
	case err != nil:
		return fmt.Errorf("location browser load failed for local machine browsing: %v", err)
	default:
	}
	switch err := checkLocalMachineBrowse(localMachineStore, localMachineResources); {
	case err != nil:
		return err
	default:
	}
	switch err := checkProfilerByteComposition(localMachineStore, localMachineResources); {
	case err != nil:
		return fmt.Errorf("hardware byte composition failed: %v", err)
	default:
	}
	noteHandle, err := findResourceHandle(store, dirResources, "note.txt", "text")
	if err != nil {
		return err
	}
	chunk, err := store.serveChildRange(noteHandle, 0, -1)
	if err != nil {
		return fmt.Errorf("cannot fetch note.txt through handle stack: %v", err)
	}
	if string(chunk) != string(noteData) {
		return fmt.Errorf("note.txt data mismatch: got %q expected %q", string(chunk), string(noteData))
	}
	textResources, err := store.loadDriverForHandle("text_driver", noteHandle)
	if err != nil {
		return fmt.Errorf("text driver load failed for note.txt: %v", err)
	}
	if _, err := findResource(textResources, "https://example.com/path", "url"); err != nil {
		return err
	}
	archiveHandle, err := findResourceHandle(store, dirResources, "archive.zip", "archive")
	if err != nil {
		return err
	}
	zipResources, err := store.loadDriverForHandle("zip_driver", archiveHandle)
	if err != nil {
		return fmt.Errorf("zip driver load failed for archive.zip: %v", err)
	}
	if _, err := findResourceHandle(store, zipResources, "inner.txt", "text"); err != nil {
		return err
	}
	nestedHandle, err := findResourceHandle(store, zipResources, "nested.zip", "archive")
	if err != nil {
		return err
	}
	nestedResources, err := store.loadDriverForHandle("zip_driver", nestedHandle)
	if err != nil {
		return fmt.Errorf("zip driver load failed for nested.zip: %v", err)
	}
	if _, err := findResourceHandle(store, nestedResources, "deep.txt", "text"); err != nil {
		return err
	}

	blobID, err := store.putBlob(archiveZip)
	if err != nil {
		return fmt.Errorf("cannot retain uploaded zip fixture: %v", err)
	}
	blobResources, err := store.loadDriverForBlob("zip_driver", blobID)
	if err != nil {
		return fmt.Errorf("zip driver load failed for uploaded blob %s: %v", blobID, err)
	}
	blobHandle, err := findResourceHandle(store, blobResources, "nested.zip", "archive")
	if err != nil {
		return err
	}
	blobSession := fmt.Sprintf("zip_driver_%s", blobID)
	if !store.stopDriver(blobSession) {
		return fmt.Errorf("cannot unload uploaded zip session %s", blobSession)
	}
	if _, ok := store.getBlob(blobID); ok {
		return fmt.Errorf("unloading uploaded zip session %s retained blob %s", blobSession, blobID)
	}
	if _, ok := store.getHandle(blobHandle.Hid); ok {
		return fmt.Errorf("unloading uploaded zip session %s retained handle %s", blobSession, blobHandle.Hid)
	}

	kubernetesHandle, err := findResourceHandle(store, rootResources, "kubernetes", "kubernetes")
	if err != nil {
		return err
	}
	contextResources, err := store.loadDriverForHandle("kubernetes_driver", kubernetesHandle)
	if err != nil {
		return fmt.Errorf("kubernetes driver load failed for the kubernetes root: %v", err)
	}
	if len(contextResources) != 3 || contextResources[0].Name != "datatree-test-context" || contextResources[1].Name != "second-test-context" || contextResources[2].Kind != "raw" {
		return fmt.Errorf("kubernetes contexts were not sorted and deduplicated: got %+v", contextResources)
	}
	contextHandle, err := findResourceHandle(store, contextResources, "datatree-test-context", "kubernetes")
	if err != nil {
		return err
	}
	namespaceResources, err := store.loadDriverForHandle("kubernetes_driver", contextHandle)
	if err != nil {
		return fmt.Errorf("kubernetes driver load failed for context datatree-test-context: %v", err)
	}
	namespaceHandle, err := findResourceHandle(store, namespaceResources, "datatree-ns", "kubernetes")
	if err != nil {
		return err
	}
	typeResources, err := store.loadDriverForHandle("kubernetes_driver", namespaceHandle)
	if err != nil {
		return fmt.Errorf("kubernetes driver load failed for namespace datatree-ns: %v", err)
	}
	podsHandle, err := findResourceHandle(store, typeResources, "pods", "kubernetes")
	if err != nil {
		return err
	}
	objectResources, err := store.loadDriverForHandle("kubernetes_driver", podsHandle)
	if err != nil {
		return fmt.Errorf("kubernetes driver load failed for resource type pods: %v", err)
	}
	podHandle, err := findResourceHandle(store, objectResources, "datatree-pod", "k8s-pod")
	if err != nil {
		return err
	}
	manifest, err := store.serveChildRange(podHandle, 0, -1)
	if err != nil {
		return fmt.Errorf("cannot fetch the pod manifest through the kubernetes handle stack: %v", err)
	}
	if string(manifest) != fakePodManifest {
		return fmt.Errorf("pod manifest mismatch: got %q expected %q", string(manifest), fakePodManifest)
	}
	manifestSlice, err := store.serveChildRange(podHandle, 9, 8)
	if err != nil {
		return fmt.Errorf("cannot fetch a pod manifest slice through the kubernetes handle stack: %v", err)
	}
	if string(manifestSlice) != fakePodManifest[9:17] {
		return fmt.Errorf("pod manifest slice mismatch: got %q expected %q", string(manifestSlice), fakePodManifest[9:17])
	}
	podView, err := store.loadDriverForHandle("k8s_pod_driver", podHandle)
	if err != nil {
		return fmt.Errorf("pod driver load failed for datatree-pod: %v", err)
	}
	if _, err := findResource(podView, "phase: Running", "kv"); err != nil {
		return err
	}
	if _, err := findResource(podView, "node: datatree-node", "kv"); err != nil {
		return err
	}
	if _, err := findResource(podView, "restarts: 2", "kv"); err != nil {
		return err
	}
	containerHandle, err := findResourceHandle(store, podView, "container main: datatree/pod:latest (ready true, restarts 2)", "json")
	if err != nil {
		return err
	}
	containerView, err := store.loadDriverForHandle("json_driver", containerHandle)
	if err != nil {
		return fmt.Errorf("json driver load failed for the pod container child: %v", err)
	}
	if _, err := findResource(containerView, "image: datatree/pod:latest", "kv"); err != nil {
		return err
	}
	manifestHandle, err := findResourceHandle(store, podView, "[manifest]", "json")
	if err != nil {
		return err
	}
	if err := checkPodManifestBrowse(store, manifestHandle); err != nil {
		return err
	}
	clusterHandle, err := findResourceHandle(store, namespaceResources, "[cluster-scoped]", "kubernetes")
	if err != nil {
		return err
	}
	clusterTypeResources, err := store.loadDriverForHandle("kubernetes_driver", clusterHandle)
	if err != nil {
		return fmt.Errorf("kubernetes driver load failed for the cluster-scoped entry: %v", err)
	}
	nodesHandle, err := findResourceHandle(store, clusterTypeResources, "nodes", "kubernetes")
	if err != nil {
		return err
	}
	nodeResources, err := store.loadDriverForHandle("kubernetes_driver", nodesHandle)
	if err != nil {
		return fmt.Errorf("kubernetes driver load failed for resource type nodes: %v", err)
	}
	if _, err := findResource(nodeResources, "datatree-node", "k8s-node"); err != nil {
		return err
	}

	objectKinds := []struct {
		typeName   string
		objectName string
		kind       string
	}{
		{typeName: "deployments.apps", objectName: "datatree-deployment", kind: "k8s-workload"},
		{typeName: "statefulsets.apps", objectName: "datatree-statefulset", kind: "k8s-workload"},
		{typeName: "daemonsets.apps", objectName: "datatree-daemonset", kind: "k8s-workload"},
		{typeName: "replicasets.apps", objectName: "datatree-replicaset", kind: "k8s-workload"},
		{typeName: "jobs.batch", objectName: "datatree-job", kind: "k8s-workload"},
		{typeName: "cronjobs.batch", objectName: "datatree-cronjob", kind: "k8s-workload"},
		{typeName: "services", objectName: "datatree-service", kind: "k8s-service"},
		{typeName: "configmaps", objectName: "datatree-config", kind: "k8s-configmap"},
		{typeName: "secrets", objectName: "blocked-secret", kind: "k8s-configmap"},
		{typeName: "widgets.example.test", objectName: "datatree-widget", kind: "json"},
	}
	for _, expected := range objectKinds {
		if err := checkKubernetesObjectKind(store, typeResources, expected.typeName, expected.objectName, expected.kind); err != nil {
			return err
		}
	}
	secretsHandle, err := findResourceHandle(store, typeResources, "secrets", "kubernetes")
	if err != nil {
		return err
	}
	secretResources, err := store.loadDriverForHandle("kubernetes_driver", secretsHandle)
	if err != nil {
		return fmt.Errorf("kubernetes driver load failed for resource type secrets: %v", err)
	}
	blockedSecretHandle, err := findResourceHandle(store, secretResources, "blocked-secret", "k8s-configmap")
	if err != nil {
		return err
	}
	blockedSecretView, err := store.loadDriverForHandle("k8s_configmap_driver", blockedSecretHandle)
	if err != nil {
		return fmt.Errorf("configmap driver load failed for blocked secret: %v", err)
	}
	if len(blockedSecretView) != 1 || blockedSecretView[0].Kind != "error" || !strings.Contains(blockedSecretView[0].Name, "access denied by the test credential") {
		return fmt.Errorf("expected the blocked secret manifest error to remain visible, got %+v", blockedSecretView)
	}

	unreachableHandle, err := findResourceHandle(store, contextResources, "second-test-context", "kubernetes")
	if err != nil {
		return err
	}
	unreachableResources, err := store.loadDriverForHandle("kubernetes_driver", unreachableHandle)
	if err != nil {
		return fmt.Errorf("kubernetes driver load failed for the unreachable context: %v", err)
	}
	if len(unreachableResources) != 1 || unreachableResources[0].Kind != "error" {
		return fmt.Errorf("expected a single error resource for the unreachable context, got %+v", unreachableResources)
	}
	if err := os.Setenv("DATATREE_KUBERNETES_MAX_RESULTS", "0"); err != nil {
		return fmt.Errorf("cannot disable the kubernetes result count limit: %v", err)
	}
	unlimitedContexts, err := store.loadDriverForHandle("kubernetes_driver", kubernetesHandle)
	os.Unsetenv("DATATREE_KUBERNETES_MAX_RESULTS")
	if err != nil {
		return fmt.Errorf("kubernetes driver load failed with an unlimited result count: %v", err)
	}
	if len(unlimitedContexts) != 3 || unlimitedContexts[0].Name != "datatree-test-context" || unlimitedContexts[1].Name != "second-test-context" || unlimitedContexts[2].Kind != "raw" {
		return fmt.Errorf("unlimited kubernetes results did not preserve every sorted context: got %+v", unlimitedContexts)
	}
	if err := os.Setenv("DATATREE_KUBERNETES_MAX_RESULTS", "1"); err != nil {
		return fmt.Errorf("cannot set bounded kubernetes result count: %v", err)
	}
	limitedContexts, err := store.loadDriverForHandle("kubernetes_driver", kubernetesHandle)
	os.Unsetenv("DATATREE_KUBERNETES_MAX_RESULTS")
	if err != nil {
		return fmt.Errorf("kubernetes driver load failed with a one-result limit: %v", err)
	}
	if len(limitedContexts) != 3 || limitedContexts[0].Name != "datatree-test-context" || limitedContexts[1].Kind != "error" || !strings.Contains(limitedContexts[1].Name, "first 1 of 2 kubeconfig contexts") || limitedContexts[2].Kind != "raw" {
		return fmt.Errorf("kubernetes result limit did not preserve one sorted result plus a truncation error: got %+v", limitedContexts)
	}
	if err := os.Setenv("DATATREE_KUBERNETES_MAX_OUTPUT_BYTES", "10"); err != nil {
		return fmt.Errorf("cannot set bounded kubernetes output size: %v", err)
	}
	boundedOutputResources, err := store.loadDriverForHandle("kubernetes_driver", kubernetesHandle)
	os.Unsetenv("DATATREE_KUBERNETES_MAX_OUTPUT_BYTES")
	if err != nil {
		return fmt.Errorf("kubernetes driver load failed with a ten-byte output limit: %v", err)
	}
	if len(boundedOutputResources) != 1 || boundedOutputResources[0].Kind != "error" || !strings.Contains(boundedOutputResources[0].Name, "DATATREE_KUBERNETES_MAX_OUTPUT_BYTES=10") {
		return fmt.Errorf("kubernetes output limit did not produce an accurate error: got %+v", boundedOutputResources)
	}

	if err := os.Setenv("DATATREE_KUBECTL", filepath.Join(fixtureDir, "missing-kubectl")); err != nil {
		return fmt.Errorf("cannot point DATATREE_KUBECTL at a missing kubectl: %v", err)
	}
	hiddenRootResources, err := store.loadRootDriver()
	if err != nil {
		return fmt.Errorf("root driver reload failed with kubectl unavailable: %v", err)
	}
	if _, err := findResource(hiddenRootResources, "kubernetes", "kubernetes"); err == nil {
		return fmt.Errorf("the kubernetes entry must be hidden when no kubectl executable can be resolved")
	}
	if err := os.Setenv("DATATREE_GH", filepath.Join(fixtureDir, "missing-gh")); err != nil {
		return fmt.Errorf("cannot point DATATREE_GH at a missing GitHub CLI: %v", err)
	}
	hiddenGitHubRootResources, err := store.loadRootDriver()
	if err != nil {
		return fmt.Errorf("root driver reload failed with GitHub CLI unavailable: %v", err)
	}
	if _, err := findResource(hiddenGitHubRootResources, "github", "github"); err == nil {
		return fmt.Errorf("the github entry must be hidden when no GitHub CLI executable can be resolved")
	}
	switch err := os.Setenv("DATATREE_AWS", filepath.Join(fixtureDir, "missing-aws")); {
	case err != nil:
		return fmt.Errorf("cannot point DATATREE_AWS at a missing AWS CLI: %v", err)
	default:
	}
	hiddenAWSRootResources, err := store.loadRootDriver()
	switch {
	case err != nil:
		return fmt.Errorf("root driver reload failed with AWS CLI unavailable: %v", err)
	default:
	}
	switch _, err := findResource(hiddenAWSRootResources, "aws", "aws"); {
	case err == nil:
		return fmt.Errorf("the aws entry must be hidden when no AWS CLI executable can be resolved")
	default:
	}
	switch err := os.Setenv("DATATREE_SYSTEM_PROFILER", filepath.Join(fixtureDir, "missing-system-profiler")); {
	case err != nil:
		return fmt.Errorf("cannot point DATATREE_SYSTEM_PROFILER at a missing executable: %v", err)
	default:
	}
	hiddenLocalMachineStore := NewStore()
	defer hiddenLocalMachineStore.stopDrivers()
	hiddenLocalMachineResources, err := hiddenLocalMachineStore.loadRootDriver()
	switch {
	case err != nil:
		return fmt.Errorf("root driver reload failed with system_profiler unavailable: %v", err)
	default:
	}
	switch _, err := findResource(hiddenLocalMachineResources, "hardware", "hardware"); {
	case err == nil:
		return fmt.Errorf("the hardware entry must be hidden when no system_profiler executable can be resolved")
	default:
	}
	switch _, err := findResource(hiddenLocalMachineResources, "usb", "usb"); {
	case err == nil:
		return fmt.Errorf("the usb entry must be hidden when no system_profiler executable can be resolved")
	default:
	}

	if err := auditKubectlCallLog(kubectlLogPath); err != nil {
		return err
	}
	if err := auditGitHubCallLog(githubLogPath); err != nil {
		return err
	}
	return nil
}
