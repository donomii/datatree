package rootdriver

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"datahumper/drivers_go/driverlib"
)

var mountedLocationRoots = []string{"/Volumes", "/mnt", "/media"}

func listRootLocations() ([]driverlib.Resource, map[string]string, error) {
	homeDirectory, err := os.UserHomeDir()
	switch {
	case err != nil:
		return nil, nil, fmt.Errorf("cannot locate HOME: %v", err)
	default:
	}
	locations := map[string]string{
		"HOME": homeDirectory,
		"/":    string(os.PathSeparator),
	}
	resources := []driverlib.Resource{
		{Name: "HOME", Kind: "directory", Size: 0},
		{Name: "/", Kind: "directory", Size: 0},
	}
	mountedResources, mountedLocations := discoverMountedLocations(mountedLocationRoots, locations)
	resources = append(resources, mountedResources...)
	for name, path := range mountedLocations {
		locations[name] = path
	}
	return resources, locations, nil
}

func discoverMountedLocations(roots []string, existing map[string]string) ([]driverlib.Resource, map[string]string) {
	resources := []driverlib.Resource{}
	locations := map[string]string{}
	seenPaths := map[string]bool{}
	for _, path := range existing {
		seenPaths[filepath.Clean(path)] = true
	}
	for _, mountRoot := range roots {
		entries, err := os.ReadDir(mountRoot)
		switch {
		case err != nil:
			continue
		default:
		}
		for _, entry := range entries {
			candidate := filepath.Join(mountRoot, entry.Name())
			info, infoErr := os.Stat(candidate)
			switch {
			case infoErr != nil || !info.IsDir() || strings.HasPrefix(entry.Name(), ".") || seenPaths[filepath.Clean(candidate)]:
				continue
			default:
			}
			name := mountedLocationName(mountRoot, entry.Name())
			seenPaths[filepath.Clean(candidate)] = true
			locations[name] = candidate
			resources = append(resources, driverlib.Resource{Name: name, Kind: "directory", Size: 0})
		}
	}
	return resources, locations
}

func mountedLocationName(mountRoot string, entryName string) string {
	switch mountRoot {
	case "/Volumes":
		return "VOLUME: " + entryName
	default:
		return "MOUNT: " + filepath.Join(mountRoot, entryName)
	}
}
