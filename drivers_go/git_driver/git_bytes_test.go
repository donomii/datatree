package gitdriver

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestFileSourcesIgnoreMetadataByteLimit(t *testing.T) {
	repository := t.TempDir()
	git, err := exec.LookPath("git")
	switch {
	case err != nil:
		t.Fatal(err)
	default:
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(git, append([]string{"-C", repository}, args...)...)
		switch data, err := cmd.CombinedOutput(); {
		case err != nil:
			t.Fatalf("fixture command %v failed: %v %s", args, err, data)
		default:
		}
	}
	run("init", "--quiet")
	payload := bytes.Repeat([]byte("payload\x00"), 1200000)
	switch err := os.WriteFile(filepath.Join(repository, "payload.bin"), payload, 0600); {
	case err != nil:
		t.Fatal(err)
	default:
	}
	run("add", "payload.bin")
	run("-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "--quiet", "-m", "Fixture")
	config := gitConfig{repository: repository, executable: git, maxOutputBytes: 32}
	for _, mode := range []string{sourceRevision, sourceIndex, sourceWorktree} {
		raw, err := readSource(config, fileSource{mode: mode, path: "payload.bin", revision: "HEAD"})
		switch {
		case err != nil || !bytes.Equal(raw, payload):
			t.Fatalf("%s returned %d of %d bytes: %v", mode, len(raw), len(payload), err)
		default:
		}
	}
}
