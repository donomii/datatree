package systemprofilerdriver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"datahumper/drivers_go/driverlib"
)

const fakeHardwareJSON = `{"SPHardwareDataType":[{"_name":"hardware_overview","chip_type":"Test Chip","machine_name":"Datatree Test Mac"}]}`
const fakeUSBJSON = `{"SPUSBHostDataType":[{"_items":[{"_name":"Test Keyboard","USBDeviceKeyProductID":"0x1234"}],"_name":"Test USB Bus","Driver":"TestUSB"},{"_name":"Test USB Bus","Driver":"TestUSB2"}]}`

func fakeSystemProfilerScript() string {
	return "#!/bin/sh\n" +
		"case \"$*\" in\n" +
		"'-listDataTypes') printf 'Available Datatypes:\\nSPHardwareDataType\\nSPUSBHostDataType\\n' ;;\n" +
		"'SPHardwareDataType -json') printf '%s' '" + fakeHardwareJSON + "' ;;\n" +
		"'SPUSBHostDataType -json') printf '%s' '" + fakeUSBJSON + "' ;;\n" +
		"*) printf 'unsupported system_profiler test invocation: %s\\n' \"$*\" >&2; exit 7 ;;\n" +
		"esac\n"
}

func findTestResource(resources []driverlib.Resource, name string, kind string) (driverlib.Resource, bool) {
	for _, resource := range resources {
		switch {
		case resource.Name == name && resource.Kind == kind:
			return resource, true
		default:
		}
	}
	return driverlib.Resource{}, false
}

func configureFakeSystemProfiler(t *testing.T) {
	fakePath := filepath.Join(t.TempDir(), "system_profiler")
	switch err := os.WriteFile(fakePath, []byte(fakeSystemProfilerScript()), 0700); {
	case err != nil:
		t.Fatalf("cannot create fake system_profiler executable %s: %v", fakePath, err)
	default:
	}
	t.Setenv("DATATREE_SYSTEM_PROFILER", fakePath)
	t.Setenv("DATATREE_SYSTEM_PROFILER_MAX_OUTPUT_BYTES", "8388608")
	t.Setenv("DATATREE_SYSTEM_PROFILER_TIMEOUT", "30s")
}

func TestBuildHardwareView(t *testing.T) {
	configureFakeSystemProfiler(t)
	resources, children, err := buildSystemProfilerView(HardwareRootState())
	switch {
	case err != nil:
		t.Fatalf("cannot build fake local hardware view: %v", err)
	case len(children) == 0:
		t.Fatal("hardware overview lost its source bytes")
	default:
	}
	switch _, found := findTestResource(resources, "model name: Datatree Test Mac", "kv"); {
	case !found:
		t.Fatalf("hardware view did not contain the model name: %+v", resources)
	default:
	}
	switch _, found := findTestResource(resources, "chip type: Test Chip", "kv"); {
	case !found:
		t.Fatalf("hardware view did not contain the chip type: %+v", resources)
	default:
	}
}

func TestBuildUSBHierarchy(t *testing.T) {
	configureFakeSystemProfiler(t)
	busResources, busChildren, err := buildSystemProfilerView(USBRootState())
	switch {
	case err != nil:
		t.Fatalf("cannot build fake local USB view: %v", err)
	default:
	}
	switch _, found := findTestResource(busResources, "Test USB Bus", "usb"); {
	case !found:
		t.Fatalf("USB view did not contain its first bus: %+v", busResources)
	default:
	}
	switch _, found := findTestResource(busResources, "Test USB Bus [2]", "usb"); {
	case !found:
		t.Fatalf("USB view did not disambiguate its duplicate bus: %+v", busResources)
	default:
	}
	busState, found := busChildren["Test USB Bus"]
	switch {
	case !found:
		t.Fatalf("USB view did not retain the first bus mount state: %v", busChildren)
	default:
	}
	deviceResources, deviceChildren, err := buildSystemProfilerView(busState)
	switch {
	case err != nil:
		t.Fatalf("cannot expand fake USB bus: %v", err)
	default:
	}
	switch _, found := findTestResource(deviceResources, "driver: TestUSB", "kv"); {
	case !found:
		t.Fatalf("USB bus did not contain its driver: %+v", deviceResources)
	default:
	}
	deviceState, found := deviceChildren["Test Keyboard"]
	switch {
	case !found:
		t.Fatalf("USB bus did not retain its keyboard mount state: %v", deviceChildren)
	default:
	}
	deviceDetails, _, err := buildSystemProfilerView(deviceState)
	switch {
	case err != nil:
		t.Fatalf("cannot expand fake USB keyboard: %v", err)
	default:
	}
	switch _, found := findTestResource(deviceDetails, "product ID: 0x1234", "kv"); {
	case !found:
		t.Fatalf("USB keyboard did not contain its product ID: %+v", deviceDetails)
	default:
	}
}

func TestSystemProfilerCommandGateRejectsOtherData(t *testing.T) {
	didPanic := false
	func() {
		defer func() {
			switch {
			case recover() != nil:
				didPanic = true
			default:
			}
		}()
		assertReadOnlySystemProfilerInvocation([]string{"SPApplicationsDataType", "-json"})
	}()
	switch {
	case !didPanic:
		t.Fatal("system_profiler command gate accepted application data, expected a panic before execution")
	default:
	}
}

func TestSystemProfilerCommandTimeout(t *testing.T) {
	fakePath := filepath.Join(t.TempDir(), "slow_system_profiler")
	script := "#!/bin/sh\nexec sleep 2\n"
	switch err := os.WriteFile(fakePath, []byte(script), 0700); {
	case err != nil:
		t.Fatalf("cannot create slow fake system_profiler executable %s: %v", fakePath, err)
	default:
	}
	t.Setenv("DATATREE_SYSTEM_PROFILER", fakePath)
	t.Setenv("DATATREE_SYSTEM_PROFILER_MAX_OUTPUT_BYTES", "8388608")
	t.Setenv("DATATREE_SYSTEM_PROFILER_TIMEOUT", "20ms")
	_, _, err := buildSystemProfilerView(HardwareRootState())
	switch {
	case err == nil:
		t.Fatal("slow system_profiler command completed, expected a timeout error")
	case !strings.Contains(err.Error(), "DATATREE_SYSTEM_PROFILER_TIMEOUT=20ms"):
		t.Fatalf("slow system_profiler returned the wrong error: %v", err)
	default:
	}
}
