package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveRootDirectoryAcceptsHTTPAndFileURLs(t *testing.T) {
	got, err := resolveRootDirectory([]string{"https://example.com/data.json"})
	if err != nil {
		t.Fatalf("resolveRootDirectory(https URL) failed: %v", err)
	} else if got != "https://example.com/data.json" {
		t.Fatalf("resolveRootDirectory(https URL) = %q, want %q", got, "https://example.com/data.json")
	} else {
		// HTTP(S) URL resolved cleanly.
	}

	tempDir := t.TempDir()
	gotDir, fileErr := resolveRootDirectory([]string{"file://" + tempDir})
	if fileErr != nil {
		t.Fatalf("resolveRootDirectory(file URL) failed: %v", fileErr)
	} else if gotDir != tempDir {
		t.Fatalf("resolveRootDirectory(file URL) = %q, want %q", gotDir, tempDir)
	} else {
		// file:// directory URL resolved cleanly.
	}

	if _, badErr := resolveRootDirectory([]string{"https://"}); badErr == nil {
		t.Fatalf("resolveRootDirectory(invalid URL) expected error, got nil")
	} else {
		// Invalid URL rejected.
	}
}

func TestFilterCLIArgsDropsMacOSPSN(t *testing.T) {
	got := filterCLIArgs([]string{"-psn_0_12345", "https://example.com/a.json"})
	if len(got) != 1 || got[0] != "https://example.com/a.json" {
		t.Fatalf("filterCLIArgs mismatch: got %v", got)
	} else {
		// LaunchServices -psn argument stripped.
	}
}

func TestOpenURLHandlerRejectsInvalidRequests(t *testing.T) {
	store := NewStore()
	defer store.stopDrivers()
	handler := openURLHandler(store)

	getReq := httptest.NewRequest(http.MethodGet, "/api/openURL", nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /api/openURL status = %d, want 405", getRec.Code)
	} else {
		// GET rejected as required.
	}

	badBody, _ := json.Marshal(OpenURLRequest{URL: "ftp://example.com/file"})
	badReq := httptest.NewRequest(http.MethodPost, "/api/openURL", bytes.NewReader(badBody))
	badRec := httptest.NewRecorder()
	handler.ServeHTTP(badRec, badReq)
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("POST /api/openURL ftp status = %d, want 400", badRec.Code)
	} else {
		// Non-HTTP URL rejected.
	}
}

func TestSelectOpenURLDriver(t *testing.T) {
	driver, err := selectOpenURLDriver("", "json")
	if err != nil || driver != "json_driver" {
		t.Fatalf("selectOpenURLDriver('', 'json') = (%q, %v), want ('json_driver', nil)", driver, err)
	} else {
		// Default driver mapped from kind.
	}
	if _, badErr := selectOpenURLDriver("root_driver", "text"); badErr == nil {
		t.Fatalf("selectOpenURLDriver('root_driver', 'text') expected error, got nil")
	} else {
		// Internal non-filter driver rejected.
	}
}

func TestResolveRootTargetAcceptsS3K8sAndFileTargets(t *testing.T) {
	rootDir, rootURL, s3Err := resolveRootTarget([]string{"s3://my-bucket/path/to/file.json"})
	if s3Err != nil {
		t.Fatalf("resolveRootTarget(s3 URL) failed: %v", s3Err)
	} else if rootDir != "" || rootURL != "s3://my-bucket/path/to/file.json" {
		t.Fatalf("resolveRootTarget(s3 URL) = (%q, %q), want ('', 's3://my-bucket/path/to/file.json')", rootDir, rootURL)
	} else {
		// S3 URL starts in location browser with saved rootURL.
	}

	if _, _, badS3Err := resolveRootTarget([]string{"s3:///"}); badS3Err == nil {
		t.Fatalf("resolveRootTarget(empty s3://) expected error, got nil")
	} else {
		// Empty s3:// target rejected.
	}

	k8sDir, k8sURL, k8sErr := resolveRootTarget([]string{"k8s://dev-ctx/default/pods/api-0"})
	if k8sErr != nil {
		t.Fatalf("resolveRootTarget(k8s URL) failed: %v", k8sErr)
	} else if k8sDir != "" || k8sURL != "k8s://dev-ctx/default/pods/api-0" {
		t.Fatalf("resolveRootTarget(k8s URL) = (%q, %q)", k8sDir, k8sURL)
	} else {
		// k8s URL accepted cleanly.
	}

	tempDir := t.TempDir()
	sampleFile := filepath.Join(tempDir, "notes.txt")
	if err := os.WriteFile(sampleFile, []byte("hello"), 0600); err != nil {
		t.Fatalf("cannot write temp file: %v", err)
	} else {
		fileDir, fileURL, fileErr := resolveRootTarget([]string{"file://" + sampleFile})
		if fileErr != nil {
			t.Fatalf("resolveRootTarget(file:// file) failed: %v", fileErr)
		} else if fileDir != tempDir || fileURL != "file://"+sampleFile {
			t.Fatalf("resolveRootTarget(file:// file) = (%q, %q), want (%q, %q)", fileDir, fileURL, tempDir, "file://"+sampleFile)
		} else {
			// file:// pointing to a file opens parent dir and preserves file:// target.
		}
	}
}

func TestMountURLNavServesJavaScript(t *testing.T) {
	mux := http.NewServeMux()
	mountURLNav(mux)
	req := httptest.NewRequest(http.MethodGet, "/static/url_nav.js", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /static/url_nav.js status = %d, want 200", rec.Code)
	} else if !strings.Contains(rec.Body.String(), "openS3InTree") || !strings.Contains(rec.Body.String(), "openTreeURL") {
		t.Fatalf("GET /static/url_nav.js missing expected URL tree navigation helpers")
	} else {
		// URL tree navigation script served cleanly.
	}
}
