package urldriver

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestValidateURL(t *testing.T) {
	cases := []struct {
		input   string
		wantOK  bool
		wantURL string
	}{
		{"https://example.com/data.json", true, "https://example.com/data.json"},
		{"HTTP://Example.COM/a?x=1", true, "http://Example.COM/a?x=1"},
		{"  https://example.com/file.zip\n", true, "https://example.com/file.zip"},
		{"", false, ""},
		{"/tmp/local/path", false, ""},
		{"ftp://example.com/file", false, ""},
		{"https://example.com/has space", false, ""},
		{"https://", false, ""},
	}
	for _, tc := range cases {
		parsed, err := ValidateURL(tc.input)
		if tc.wantOK {
			if err != nil {
				t.Fatalf("ValidateURL(%q) unexpected error: %v", tc.input, err)
			} else if parsed.String() != tc.wantURL {
				t.Fatalf("ValidateURL(%q) = %q, want %q", tc.input, parsed.String(), tc.wantURL)
			} else {
				// Valid case matched expected normalized URL.
			}
		} else {
			if err == nil {
				t.Fatalf("ValidateURL(%q) expected error, got %q", tc.input, parsed.String())
			} else {
				// Invalid case rejected as expected.
			}
		}
	}
}

func TestFetchURLClassifiesAndReadsBytes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/payload":
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_, _ = w.Write([]byte(`{"hello":"world"}`))
		case "/missing":
			http.Error(w, "not found", http.StatusNotFound)
		default:
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("plain text"))
		}
	}))
	defer server.Close()

	res, err := FetchURL(server.URL+"/payload", 1024*1024, 5*time.Second)
	if err != nil {
		t.Fatalf("FetchURL(/payload) failed: %v", err)
	} else if res.Kind != "json" || string(res.Body) != `{"hello":"world"}` {
		t.Fatalf("FetchURL(/payload) = (%q, %q), want (json, json body)", res.Kind, string(res.Body))
	} else {
		// Content-Type classified extensionless JSON payload.
	}

	if _, err := FetchURL(server.URL+"/missing", 1024*1024, 5*time.Second); err == nil {
		t.Fatalf("FetchURL(/missing) expected HTTP error, got nil")
	} else {
		// 404 rejected with error.
	}

	if _, err := FetchURL(server.URL+"/other", 4, 5*time.Second); err == nil {
		t.Fatalf("FetchURL(/other) expected size limit error, got nil")
	} else {
		// Oversized response rejected with error.
	}
}
