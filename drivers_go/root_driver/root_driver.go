// drivers_go/root_driver/root_driver.go
package rootdriver

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"datahumper/drivers_go/driverlib"
	systemprofilerdriver "datahumper/drivers_go/system_profiler_driver"
)

// root_driver lists a selected directory or, when no directory was selected,
// lists filesystem locations. It stays alive serving child ranges: files as
// their bytes and directories as their absolute path.

// kubernetesEntryName is the virtual root resource that mounts the kubernetes
// driver. It is emitted only when a kubectl executable can be resolved.
const kubernetesEntryName = "kubernetes"

const kubernetesEntryKey = "\x00datatree:virtual:kubernetes"

// kubernetesRootState is the mount state served for the virtual kubernetes
// entry: an empty state, meaning the kubernetes driver's root level.
const kubernetesRootState = "{}"

// githubEntryName is the virtual root resource that mounts the GitHub driver.
// It is emitted only when the gh executable can be resolved.
const githubEntryName = "github"

const githubEntryKey = "\x00datatree:virtual:github"

// githubRootState is the empty GitHub mount state, which lists the authenticated
// user's authored and review-requested pull request views.
const githubRootState = "{}"

const awsEntryName = "aws"

const awsEntryKey = "\x00datatree:virtual:aws"

const awsRootState = "{}"

const hardwareEntryName = "hardware"

const hardwareEntryKey = "\x00datatree:virtual:hardware"

const usbEntryName = "usb"

const usbEntryKey = "\x00datatree:virtual:usb"

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

// resolveGitHubCLI locates the gh executable. DATATREE_GH overrides the search
// so tests can substitute a fake GitHub CLI.
func resolveGitHubCLI() (string, error) {
	candidate := os.Getenv("DATATREE_GH")
	switch {
	case candidate == "":
		candidate = "gh"
	default:
	}
	githubPath, err := exec.LookPath(candidate)
	switch {
	case err != nil:
		return "", fmt.Errorf("cannot find a usable GitHub CLI executable: looked for %q (set DATATREE_GH to override), got: %v", candidate, err)
	default:
		return githubPath, nil
	}
}

func resolveAWSCLI() (string, error) {
	candidate := os.Getenv("DATATREE_AWS")
	switch {
	case candidate == "":
		candidate = "aws"
	default:
	}
	awsPath, err := exec.LookPath(candidate)
	switch {
	case err != nil:
		return "", fmt.Errorf("cannot find a usable AWS CLI executable: looked for %q (set DATATREE_AWS to override), got: %v", candidate, err)
	default:
		return awsPath, nil
	}
}

func Run() {
	defer driverlib.RecoverPanic("Root driver")

	enc := json.NewEncoder(os.Stdout)
	dec := json.NewDecoder(os.Stdin)

	var rootDir string
	var locations map[string]string
	var resources []driverlib.Resource
	switch len(os.Args) {
	case 1:
		var locationErr error
		resources, locations, locationErr = listRootLocations()
		switch {
		case locationErr != nil:
			fmt.Fprintf(os.Stderr, "Failed to list filesystem locations: expected HOME and / plus readable mounted volumes, got error: %v\n", locationErr)
			enc.Encode(driverlib.ResourcesMsg{Resources: nil})
			return
		default:
		}
	case 2:
		rootDir = os.Args[1]
	default:
		fmt.Fprintf(os.Stderr, "Failed to select the root directory: expected zero or one directory argument, got %d arguments\n", len(os.Args)-1)
		enc.Encode(driverlib.ResourcesMsg{Resources: nil})
		return
	}
	switch rootDir {
	case "":
	default:
		var err error
		rootDir, err = filepath.Abs(rootDir)
		switch {
		case err != nil:
			fmt.Fprintf(os.Stderr, "Failed to resolve root directory %s to an absolute path: %v\n", rootDir, err)
			enc.Encode(driverlib.ResourcesMsg{Resources: nil})
			return
		default:
		}
		var listErr error
		resources, listErr = driverlib.ListDirectory(rootDir)
		switch {
		case listErr != nil:
			fmt.Fprintf(os.Stderr, "Failed to read root directory %s: %v\n", rootDir, listErr)
			enc.Encode(driverlib.ResourcesMsg{Resources: nil})
			return
		default:
		}
	}

	// Offer the virtual kubernetes entry only when kubectl is usable
	kubectlPath, kubectlErr := resolveKubectl()
	switch {
	case kubectlErr == nil:
		fmt.Fprintf(os.Stderr, "Root driver offering kubernetes entry via %s\n", kubectlPath)
		resources = append([]driverlib.Resource{{Name: kubernetesEntryName, Kind: "kubernetes", Size: 0, Key: kubernetesEntryKey}}, resources...)
	default:
		fmt.Fprintf(os.Stderr, "Root driver skipping kubernetes entry: %v\n", kubectlErr)
	}

	// Offer the virtual github entry only when gh is usable
	githubPath, githubErr := resolveGitHubCLI()
	switch {
	case githubErr == nil:
		fmt.Fprintf(os.Stderr, "Root driver offering github entry via %s\n", githubPath)
		resources = append([]driverlib.Resource{{Name: githubEntryName, Kind: "github", Size: 0, Key: githubEntryKey}}, resources...)
	default:
		fmt.Fprintf(os.Stderr, "Root driver skipping github entry: %v\n", githubErr)
	}

	awsPath, awsErr := resolveAWSCLI()
	switch {
	case awsErr == nil:
		fmt.Fprintf(os.Stderr, "Root driver offering aws entry via %s\n", awsPath)
		resources = append([]driverlib.Resource{{Name: awsEntryName, Kind: "aws", Size: 0, Key: awsEntryKey}}, resources...)
	default:
		fmt.Fprintf(os.Stderr, "Root driver skipping aws entry: %v\n", awsErr)
	}

	systemProfilerPath, systemProfilerErr := systemprofilerdriver.ResolveExecutable()
	switch {
	case systemProfilerErr == nil:
		fmt.Fprintf(os.Stderr, "Root driver offering hardware and usb entries via %s\n", systemProfilerPath)
		resources = append([]driverlib.Resource{{Name: usbEntryName, Kind: "usb", Size: 0, Key: usbEntryKey}}, resources...)
		resources = append([]driverlib.Resource{{Name: hardwareEntryName, Kind: "hardware", Size: 0, Key: hardwareEntryKey}}, resources...)
	default:
		fmt.Fprintf(os.Stderr, "Root driver skipping hardware and usb entries: %v\n", systemProfilerErr)
	}

	// Send initial resources
	switch err := enc.Encode(driverlib.ResourcesMsg{Resources: resources}); {
	case err != nil:
		fmt.Fprintf(os.Stderr, "Failed to encode resources: %v\n", err)
		return
	default:
	}

	// Handle range requests for child files
	for {
		var req driverlib.RangeReq
		switch err := dec.Decode(&req); {
		case err != nil:
			// EOF or connection closed
			return
		default:
		}
		switch {
		case req.Cmd != "range":
			continue
		default:
		}
		switch {
		case req.Child == "":
			// No child specified, can't serve anything meaningful
			driverlib.SendRange(enc, nil)
			continue
		default:
		}
		switch {
		case req.Child == kubernetesEntryKey:
			// Virtual kubernetes entry: serve the root mount state so the
			// kubernetes driver starts at its context list
			driverlib.SendRange(enc, []byte(kubernetesRootState))
			continue
		default:
		}
		switch {
		case req.Child == githubEntryKey:
			// Virtual github entry: serve the root mount state so the GitHub
			// driver starts at the authenticated user's pull request lists
			driverlib.SendRange(enc, []byte(githubRootState))
			continue
		default:
		}
		switch {
		case req.Child == awsEntryKey:
			driverlib.SendRange(enc, []byte(awsRootState))
			continue
		default:
		}
		switch {
		case req.Child == hardwareEntryKey:
			driverlib.SendRange(enc, systemprofilerdriver.HardwareRootState())
			continue
		default:
		}
		switch {
		case req.Child == usbEntryKey:
			driverlib.SendRange(enc, systemprofilerdriver.USBRootState())
			continue
		default:
		}
		switch rootDir {
		case "":
			locationPath, ok := locations[req.Child]
			switch {
			case ok:
				driverlib.SendRange(enc, []byte(locationPath))
			default:
				driverlib.SendRange(enc, nil)
			}
			continue
		default:
		}

		// Security check: ensure path is still within the root directory
		cleanPath, err := filepath.Abs(filepath.Join(rootDir, req.Child))
		switch {
		case err != nil || !driverlib.InsideBase(rootDir, cleanPath):
			driverlib.SendRange(enc, nil)
		default:
			// Directories are served as their path bytes, files as bytes
			driverlib.ServeFileChild(enc, cleanPath, req.Offset, req.Length)
		}
	}
}
