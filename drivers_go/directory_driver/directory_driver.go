// drivers_go/directory_driver/directory_driver.go
package directorydriver

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"datahumper/drivers_go/driverlib"
)

// directory_driver interprets the upstream bytes as an absolute directory
// path, lists that directory (non-hidden entries, classified by extension),
// and stays alive serving child ranges: files as their bytes, directories as
// their absolute path so another directory_driver can mount them.

func Run() {
	defer driverlib.RecoverPanic("Directory driver")

	enc := json.NewEncoder(os.Stdout)
	dec := json.NewDecoder(os.Stdin)

	// First, ask for the directory path from upstream
	pathBytes, fetchErr := driverlib.FetchUpstreamBytes(enc, dec)
	switch {
	case fetchErr != nil:
		fmt.Fprintf(os.Stderr, "Failed to fetch the directory path: %v\n", fetchErr)
		return
	case len(pathBytes) == 0:
		fmt.Fprintf(os.Stderr, "No directory path provided\n")
		enc.Encode(driverlib.ResourcesMsg{Resources: nil})
		return
	default:
	}

	dirPath := strings.TrimSpace(string(pathBytes))
	fmt.Fprintf(os.Stderr, "Directory driver reading directory: %s\n", dirPath)
	basePath, err := filepath.Abs(dirPath)
	switch {
	case err != nil:
		fmt.Fprintf(os.Stderr, "Failed to resolve directory path %s: %v\n", dirPath, err)
		enc.Encode(driverlib.ResourcesMsg{Resources: nil})
		return
	default:
	}
	dirPath = basePath

	// Check if directory exists
	info, err := os.Stat(dirPath)
	switch {
	case err != nil || !info.IsDir():
		fmt.Fprintf(os.Stderr, "Invalid directory: %s\n", dirPath)
		enc.Encode(driverlib.ResourcesMsg{Resources: nil})
		return
	default:
	}

	// Read directory contents, skipping hidden entries
	resources, listErr := driverlib.ListDirectory(dirPath)
	switch {
	case listErr != nil:
		fmt.Fprintf(os.Stderr, "Failed to read directory %s: %v\n", dirPath, listErr)
		enc.Encode(driverlib.ResourcesMsg{Resources: nil})
		return
	default:
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

		// Security check: ensure path is still within the directory
		cleanPath, err := filepath.Abs(filepath.Join(dirPath, req.Child))
		switch {
		case err != nil || !driverlib.InsideBase(dirPath, cleanPath):
			driverlib.SendRange(enc, nil)
		default:
			// Directories are served as their path bytes, files as bytes
			driverlib.ServeFileChild(enc, cleanPath, req.Offset, req.Length)
		}
	}
}
