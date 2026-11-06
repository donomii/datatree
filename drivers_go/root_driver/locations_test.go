package rootdriver

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverMountedLocations(t *testing.T) {
	mountRoot := t.TempDir()
	volumePath := filepath.Join(mountRoot, "Backup")
	filePath := filepath.Join(mountRoot, "not-a-drive")
	hiddenPath := filepath.Join(mountRoot, ".hidden")
	if err := os.Mkdir(volumePath, 0700); err != nil {
		t.Fatalf("cannot create mounted-location fixture %s: %v", volumePath, err)
	} else {
	}
	if err := os.WriteFile(filePath, []byte("not a directory"), 0600); err != nil {
		t.Fatalf("cannot create mounted-location file fixture %s: %v", filePath, err)
	} else {
	}
	if err := os.Mkdir(hiddenPath, 0700); err != nil {
		t.Fatalf("cannot create hidden mounted-location fixture %s: %v", hiddenPath, err)
	} else {
	}
	resources, locations := discoverMountedLocations([]string{mountRoot}, map[string]string{"HOME": t.TempDir()})
	expectedName := "MOUNT: " + volumePath
	switch {
	case len(resources) != 1:
		t.Fatalf("mounted-location resource count mismatch: got %d expected 1: %+v", len(resources), resources)
	case resources[0].Name != expectedName || resources[0].Kind != "directory":
		t.Fatalf("mounted-location resource mismatch: got %+v expected name %q kind directory", resources[0], expectedName)
	case locations[expectedName] != volumePath:
		t.Fatalf("mounted-location path mismatch: got %q expected %q", locations[expectedName], volumePath)
	default:
	}
}

func TestMountedLocationNameUsesVolumeLabelOnMacOS(t *testing.T) {
	got := mountedLocationName("/Volumes", "Backup")
	switch {
	case got != "VOLUME: Backup":
		t.Fatalf("macOS volume label mismatch: got %q expected %q", got, "VOLUME: Backup")
	default:
	}
}
