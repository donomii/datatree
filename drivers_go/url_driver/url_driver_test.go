package urldriver

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
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
	allowPrivateIPsForTest = true
	t.Cleanup(func() { allowPrivateIPsForTest = false })

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

func TestIsBlockedIP(t *testing.T) {
	blocked := []string{
		"127.0.0.1",
		"127.255.255.254",
		"::1",
		"::ffff:127.0.0.1",
		"10.0.0.1",
		"172.16.0.1",
		"192.168.1.1",
		"169.254.169.254",
		"fe80::1",
		"fc00::1",
		"0.0.0.0",
		"::",
		"224.0.0.1",
	}
	for _, raw := range blocked {
		ip := net.ParseIP(raw)
		if ip == nil {
			t.Fatalf("net.ParseIP(%q) returned nil", raw)
		}
		if !isBlockedIP(ip) {
			t.Fatalf("isBlockedIP(%q) = false, want true", raw)
		}
	}

	allowed := []string{
		"93.184.216.34",
		"8.8.8.8",
		"2606:2800:220:1:248:1893:25c8:1946",
	}
	for _, raw := range allowed {
		ip := net.ParseIP(raw)
		if ip == nil {
			t.Fatalf("net.ParseIP(%q) returned nil", raw)
		}
		if isBlockedIP(ip) {
			t.Fatalf("isBlockedIP(%q) = true, want false", raw)
		}
	}
}

func TestFetchURLBlocksSSRFTargets(t *testing.T) {
	allowPrivateIPsForTest = false

	hit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		_, _ = w.Write([]byte("should never be reached"))
	}))
	defer server.Close()

	targets := []string{
		server.URL + "/secret",
		"http://127.0.0.1:80/",
		"http://[::1]:80/",
		"http://169.254.169.254/latest/meta-data/",
		"http://10.0.0.1/",
		"http://172.16.0.1/",
		"http://192.168.1.1/",
		"http://0.0.0.0/",
	}
	for _, target := range targets {
		_, err := FetchURL(target, 1024, 2*time.Second)
		if err == nil {
			t.Fatalf("FetchURL(%q) unexpectedly succeeded", target)
		}
		if !strings.Contains(err.Error(), "blocked") {
			t.Fatalf("FetchURL(%q) error = %q, want error mentioning blocked IP", target, err.Error())
		}
	}
	if hit {
		t.Fatal("loopback httptest server was contacted despite SSRF blocking")
	}
}

func TestCheckURLRedirectBlocksPrivateAndNonHTTPTargets(t *testing.T) {
	allowPrivateIPsForTest = false

	blockedRedirects := []string{
		"http://127.0.0.1:8080/internal",
		"http://169.254.169.254/latest/meta-data/",
		"http://10.0.0.5/admin",
		"http://[::1]/secret",
		"file:///etc/passwd",
	}
	for _, target := range blockedRedirects {
		req, err := http.NewRequest(http.MethodGet, target, nil)
		if err != nil {
			t.Fatalf("http.NewRequest(%q) failed: %v", target, err)
		}
		if err := checkURLRedirect(req, []*http.Request{req}); err == nil {
			t.Fatalf("checkURLRedirect(%q) unexpectedly allowed redirect", target)
		}
	}
}
