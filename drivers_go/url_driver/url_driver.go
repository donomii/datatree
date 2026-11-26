// drivers_go/url_driver/url_driver.go
package urldriver

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"datahumper/drivers_go/driverlib"
)

const defaultMaxURLBytes int64 = 64 * 1024 * 1024
const defaultURLTimeout = 30 * time.Second
const maxURLLength = 8192
const maxURLRedirects = 10

// allowPrivateIPsForTest allows unit tests using httptest.NewServer on loopback
// addresses to opt out of private/loopback IP blocking. Production leaves this false.
var allowPrivateIPsForTest bool

type FetchResult struct {
	FinalURL    string
	StatusCode  int
	Status      string
	ContentType string
	DisplayName string
	Kind        string
	Body        []byte
}

func Run() {
	driverlib.Run("URL driver", listURL)
}

func IsHTTPURL(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	lower := strings.ToLower(trimmed)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return false
	} else {
		_, err := ValidateURL(trimmed)
		return err == nil
	}
}

func ValidateURL(raw string) (*url.URL, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("cannot open URL: expected an http:// or https:// URL, got empty input")
	} else if len(trimmed) > maxURLLength {
		return nil, fmt.Errorf("cannot open URL: length %d exceeds maximum %d bytes", len(trimmed), maxURLLength)
	} else if strings.ContainsAny(trimmed, " \t\r\n\x00") {
		return nil, fmt.Errorf("cannot open URL %q: URL must not contain whitespace or NUL bytes", trimmed)
	} else {
		parsed, err := url.Parse(trimmed)
		if err != nil {
			return nil, fmt.Errorf("cannot parse URL %q: expected a valid http:// or https:// URL, got error: %v", trimmed, err)
		} else {
			scheme := strings.ToLower(parsed.Scheme)
			if scheme != "http" && scheme != "https" {
				return nil, fmt.Errorf("cannot open URL %q: expected http or https scheme, got %q", trimmed, parsed.Scheme)
			} else if parsed.Host == "" {
				return nil, fmt.Errorf("cannot open URL %q: expected a host name", trimmed)
			} else {
				parsed.Scheme = scheme
				return parsed, nil
			}
		}
	}
}

func ResolveMaxBytes() int64 {
	raw := os.Getenv("DATATREE_MAX_URL_BYTES")
	if raw == "" {
		raw = os.Getenv("DATATREE_MAX_UPLOAD_BYTES")
	} else {
		// DATATREE_MAX_URL_BYTES takes precedence over DATATREE_MAX_UPLOAD_BYTES.
	}
	if raw == "" {
		return defaultMaxURLBytes
	} else {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value <= 0 {
			return defaultMaxURLBytes
		} else {
			return value
		}
	}
}

func ResolveTimeout() time.Duration {
	raw := os.Getenv("DATATREE_URL_TIMEOUT")
	if raw == "" {
		return defaultURLTimeout
	} else {
		timeout, err := time.ParseDuration(raw)
		if err != nil || timeout <= 0 {
			return defaultURLTimeout
		} else {
			return timeout
		}
	}
}

func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified()
}

func resolveAllowedIPs(ctx context.Context, host string) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		if !allowPrivateIPsForTest && isBlockedIP(ip) {
			return nil, fmt.Errorf("blocked request to non-public IP address %s", ip.String())
		}
		return []net.IP{ip}, nil
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("host %q did not resolve to any IP address", host)
	}
	allowed := make([]net.IP, 0, len(addrs))
	for _, addr := range addrs {
		if !allowPrivateIPsForTest && isBlockedIP(addr.IP) {
			return nil, fmt.Errorf("blocked request to host %q resolving to non-public IP address %s", host, addr.IP.String())
		}
		allowed = append(allowed, addr.IP)
	}
	return allowed, nil
}

func safeDialContext(dialer *net.Dialer) func(ctx context.Context, network, address string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := resolveAllowedIPs(ctx, host)
		if err != nil {
			return nil, err
		}
		var lastErr error
		for _, ip := range ips {
			conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if dialErr == nil {
				return conn, nil
			}
			lastErr = dialErr
		}
		return nil, lastErr
	}
}

func checkURLRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxURLRedirects {
		return fmt.Errorf("stopped after %d redirects", maxURLRedirects)
	}
	if req.URL == nil {
		return fmt.Errorf("redirect target has empty URL")
	}
	parsed, err := ValidateURL(req.URL.String())
	if err != nil {
		return fmt.Errorf("blocked redirect target: %v", err)
	}
	if _, err := resolveAllowedIPs(req.Context(), parsed.Hostname()); err != nil {
		return fmt.Errorf("blocked redirect target %q: %v", parsed.String(), err)
	}
	return nil
}

func newSafeHTTPClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = safeDialContext(dialer)
	return &http.Client{
		Timeout:       timeout,
		Transport:     transport,
		CheckRedirect: checkURLRedirect,
	}
}

func FetchURL(rawURL string, maxBytes int64, timeout time.Duration) (FetchResult, error) {
	parsed, err := ValidateURL(rawURL)
	if err != nil {
		return FetchResult{}, err
	} else {
		if maxBytes <= 0 {
			maxBytes = defaultMaxURLBytes
		} else {
			// Retain caller's positive byte bound.
		}
		if timeout <= 0 {
			timeout = defaultURLTimeout
		} else {
			// Retain caller's positive request timeout.
		}
		client := newSafeHTTPClient(timeout)
		req, reqErr := http.NewRequest(http.MethodGet, parsed.String(), nil)
		if reqErr != nil {
			return FetchResult{}, fmt.Errorf("cannot build HTTP GET request for %q: %v", parsed.String(), reqErr)
		} else {
			req.Header.Set("User-Agent", "Datatree/1.0")
			resp, respErr := client.Do(req)
			if respErr != nil {
				return FetchResult{}, fmt.Errorf("cannot fetch URL %q: expected HTTP response, got network error: %v", parsed.String(), respErr)
			} else {
				defer resp.Body.Close()
				if resp.StatusCode < 200 || resp.StatusCode >= 400 {
					return FetchResult{}, fmt.Errorf("cannot open URL %q: expected HTTP 2xx status, got %s", parsed.String(), resp.Status)
				} else {
					body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
					if readErr != nil {
						return FetchResult{}, fmt.Errorf("cannot read response body from URL %q: %v", parsed.String(), readErr)
					} else if int64(len(body)) > maxBytes {
						return FetchResult{}, fmt.Errorf("response from URL %q exceeds %d-byte limit (read more than %d bytes)", parsed.String(), maxBytes, maxBytes)
					} else {
						finalURL := parsed
						if resp.Request != nil && resp.Request.URL != nil {
							finalURL = resp.Request.URL
						} else {
							// Keep initial parsed URL when request URL is absent.
						}
						contentType := resp.Header.Get("Content-Type")
						displayName := displayNameForURL(finalURL)
						kind := classifyFetchedResource(displayName, contentType, body)
						return FetchResult{
							FinalURL:    finalURL.String(),
							StatusCode:  resp.StatusCode,
							Status:      resp.Status,
							ContentType: contentType,
							DisplayName: displayName,
							Kind:        kind,
							Body:        body,
						}, nil
					}
				}
			}
		}
	}
}

func displayNameForURL(parsed *url.URL) string {
	base := path.Base(parsed.Path)
	if base == "" || base == "." || base == "/" {
		return parsed.Host
	} else {
		return base
	}
}

func kindFromContentType(header string) (string, bool) {
	if strings.TrimSpace(header) == "" {
		return "", false
	} else {
		mediaType, _, err := mime.ParseMediaType(header)
		if err != nil {
			return "", false
		} else {
			lower := strings.ToLower(mediaType)
			switch lower {
			case "application/json", "text/json", "application/problem+json", "application/ld+json":
				return "json", true
			case "application/x-ndjson", "application/jsonl":
				return "ndjson", true
			case "text/html", "application/xhtml+xml":
				return "html", true
			case "application/zip", "application/x-zip-compressed":
				return "archive", true
			case "application/x-tar":
				return "tar", true
			case "application/gzip", "application/x-gzip":
				return "gzip", true
			case "application/x-bzip2":
				return "bzip2", true
			case "application/x-xz":
				return "xz", true
			case "application/zstd":
				return "zstd", true
			case "application/pdf":
				return "pdf", true
			case "text/csv":
				return "csv", true
			case "text/tab-separated-values":
				return "tsv", true
			case "application/xml", "text/xml", "image/svg+xml":
				return "xml", true
			case "application/yaml", "text/yaml", "application/x-yaml", "text/x-yaml":
				return "yaml", true
			case "application/toml", "text/toml":
				return "toml", true
			case "text/plain", "text/markdown":
				return "text", true
			default:
				if strings.HasPrefix(lower, "image/") {
					return "image", true
				} else if strings.HasSuffix(lower, "+json") {
					return "json", true
				} else if strings.HasSuffix(lower, "+xml") {
					return "xml", true
				} else {
					return "", false
				}
			}
		}
	}
}

func classifyFetchedResource(displayName string, contentType string, body []byte) string {
	if extKind, ok := driverlib.FormatKindForName(displayName); ok {
		return extKind
	} else if ctKind, ok := kindFromContentType(contentType); ok {
		return ctKind
	} else {
		return driverlib.DetectKind(body)
	}
}

func listURL(raw []byte) ([]driverlib.Resource, map[string][]byte, error) {
	result, err := FetchURL(string(raw), ResolveMaxBytes(), ResolveTimeout())
	if err != nil {
		return nil, nil, err
	} else {
		var resources []driverlib.Resource
		resources = driverlib.AppendKV(resources, "url", result.FinalURL)
		resources = driverlib.AppendKV(resources, "status", result.Status)
		resources = driverlib.AppendKV(resources, "content-type", result.ContentType)
		resources = driverlib.AppendKV(resources, "size", strconv.Itoa(len(result.Body)))
		children := map[string][]byte{
			"body": result.Body,
		}
		resources = append(resources, driverlib.Resource{
			Name:    result.DisplayName,
			Kind:    result.Kind,
			Size:    int64(len(result.Body)),
			Key:     "body",
			Payload: result.Body,
		})
		return resources, children, nil
	}
}
