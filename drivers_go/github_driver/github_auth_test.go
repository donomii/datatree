package githubdriver

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestExpiredGitHubSessionRefreshesWithoutAddingScopes(t *testing.T) {
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_HOST", "github.com")
	executable := filepath.Join(t.TempDir(), "gh")
	script := "#!/bin/sh\ncase \"$*\" in 'auth refresh --hostname github.com') exit 0 ;; *) exit 97 ;; esac\n"
	switch err := os.WriteFile(executable, []byte(script), 0700); {
	case err != nil:
		t.Fatal(err)
	default:
	}
	calls := 0
	raw, err := withGitHubLogin(githubConfig{executable: executable, maxOutputBytes: 4096}, func() ([]byte, error) {
		calls++
		switch calls {
		case 1:
			return nil, fmt.Errorf("HTTP 401: Bad credentials")
		default:
			return []byte("actual bytes"), nil
		}
	})
	switch {
	case err != nil || string(raw) != "actual bytes" || calls != 2:
		t.Fatalf("refresh failed to resume read: %q calls=%d err=%v", raw, calls, err)
	default:
	}
}
