package directorydriver

import (
	"os"
	"path/filepath"
	"testing"

	"datahumper/drivers_go/driverlib"
)

func TestResolveChildPathRejectsSymlinkEscapeAndTraversal(t *testing.T) {
	root := t.TempDir()
	baseDir := filepath.Join(root, "base")
	outsideDir := filepath.Join(root, "outside")
	if err := os.MkdirAll(filepath.Join(baseDir, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outsideDir, 0755); err != nil {
		t.Fatal(err)
	}

	insideFile := filepath.Join(baseDir, "sub", "ok.txt")
	if err := os.WriteFile(insideFile, []byte("inside"), 0600); err != nil {
		t.Fatal(err)
	}
	outsideFile := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}

	// Symlink inside base pointing to another file inside base (allowed).
	inBaseLink := filepath.Join(baseDir, "link_inside.txt")
	if err := os.Symlink(insideFile, inBaseLink); err != nil {
		t.Fatal(err)
	}

	// Symlink inside base pointing to a file outside base (must be rejected).
	escapeFileLink := filepath.Join(baseDir, "escape_file.txt")
	if err := os.Symlink(outsideFile, escapeFileLink); err != nil {
		t.Fatal(err)
	}

	// Symlink inside base pointing to a directory outside base (must be rejected).
	escapeDirLink := filepath.Join(baseDir, "escape_dir")
	if err := os.Symlink(outsideDir, escapeDirLink); err != nil {
		t.Fatal(err)
	}

	// Broken symlink pointing outside base (must be rejected).
	brokenLink := filepath.Join(baseDir, "broken_escape.txt")
	if err := os.Symlink(filepath.Join(outsideDir, "nonexistent.txt"), brokenLink); err != nil {
		t.Fatal(err)
	}

	// Valid regular file and valid in-base symlink succeed.
	if got, ok := resolveChildPath(baseDir, filepath.Join("sub", "ok.txt")); !ok || got == "" {
		t.Fatalf("resolveChildPath(sub/ok.txt) = (%q, %v), want allowed", got, ok)
	}
	if got, ok := resolveChildPath(baseDir, "link_inside.txt"); !ok || got == "" {
		t.Fatalf("resolveChildPath(link_inside.txt) = (%q, %v), want allowed", got, ok)
	}

	// Escapes must be rejected by both resolveChildPath and driverlib.InsideBase.
	rejected := []string{
		"escape_file.txt",
		"escape_dir",
		filepath.Join("escape_dir", "secret.txt"),
		"broken_escape.txt",
		filepath.Join("..", "outside", "secret.txt"),
	}
	for _, child := range rejected {
		if got, ok := resolveChildPath(baseDir, child); ok {
			t.Fatalf("resolveChildPath(%q) unexpectedly allowed -> %q", child, got)
		}
		target := filepath.Join(baseDir, child)
		if driverlib.InsideBase(baseDir, target) {
			t.Fatalf("driverlib.InsideBase(%q, %q) unexpectedly returned true", baseDir, target)
		}
	}
}
