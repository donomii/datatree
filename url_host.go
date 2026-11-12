package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	urldriver "datahumper/drivers_go/url_driver"
)

//go:embed web/url_nav.js
var urlNavScript []byte

func mountURLNav(mux *http.ServeMux) {
	mux.HandleFunc("GET /static/url_nav.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = w.Write(urlNavScript)
	})
}

func resolveExistingAncestorDirectory(targetPath string) (string, bool) {
	current := targetPath
	for {
		info, err := os.Stat(current)
		if err == nil && info.IsDir() {
			return current, true
		} else {
			parent := filepath.Dir(current)
			if parent == current {
				return "", false
			} else {
				current = parent
			}
		}
	}
}

func validateTreeTargetURL(raw string, expectedScheme string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if strings.ContainsAny(trimmed, "\r\n\x00") {
		return "", fmt.Errorf("cannot open %s:// URL %q: URL must not contain newlines or NUL bytes", expectedScheme, raw)
	} else {
		parsed, err := url.Parse(trimmed)
		if err != nil {
			return "", fmt.Errorf("cannot parse %s:// URL %q: %v", expectedScheme, raw, err)
		} else if strings.ToLower(parsed.Scheme) != expectedScheme {
			return "", fmt.Errorf("cannot open URL %q: expected %s scheme, got %q", raw, expectedScheme, parsed.Scheme)
		} else if parsed.Host == "" && strings.Trim(parsed.Path, "/") == "" {
			return "", fmt.Errorf("cannot open %s:// URL %q: expected a target path or bucket/context host", expectedScheme, raw)
		} else {
			parsed.Scheme = expectedScheme
			return parsed.String(), nil
		}
	}
}

func resolveRootTarget(args []string) (string, string, error) {
	var requested string
	switch len(args) {
	case 0:
		return "", "", nil
	case 1:
		requested = args[0]
	default:
		return "", "", fmt.Errorf("cannot select the directory or URL to open: expected zero or one argument, got %d arguments: %q", len(args), args)
	}
	trimmed := strings.TrimSpace(requested)
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		parsedURL, err := urldriver.ValidateURL(trimmed)
		if err != nil {
			return "", "", err
		} else {
			return parsedURL.String(), parsedURL.String(), nil
		}
	} else if strings.HasPrefix(lower, "s3://") {
		validS3, err := validateTreeTargetURL(trimmed, "s3")
		if err != nil {
			return "", "", err
		} else {
			return "", validS3, nil
		}
	} else if strings.HasPrefix(lower, "k8s://") {
		validK8s, err := validateTreeTargetURL(trimmed, "k8s")
		if err != nil {
			return "", "", err
		} else {
			return "", validK8s, nil
		}
	} else if strings.HasPrefix(lower, "file://") {
		parsedFileURL, err := url.Parse(trimmed)
		if err != nil || parsedFileURL.Path == "" {
			return "", "", fmt.Errorf("cannot open file URL %q: expected a valid file:// path, got error: %v", requested, err)
		} else {
			absPath, absErr := filepath.Abs(parsedFileURL.Path)
			if absErr != nil {
				return "", "", fmt.Errorf("cannot resolve file URL %q: %v", requested, absErr)
			} else {
				info, statErr := os.Stat(absPath)
				if statErr == nil && info.IsDir() {
					return absPath, "", nil
				} else if ancestorDir, ok := resolveExistingAncestorDirectory(absPath); ok {
					return ancestorDir, "file://" + absPath, nil
				} else {
					return "", "", fmt.Errorf("cannot open file URL %q: expected an existing directory or readable path, got stat error: %v", absPath, statErr)
				}
			}
		}
	} else {
		dir, err := resolveRootDirectory(args)
		if err == nil {
			return dir, "", nil
		} else {
			absPath, absErr := filepath.Abs(trimmed)
			if absErr == nil {
				if info, statErr := os.Stat(absPath); statErr == nil && !info.IsDir() {
					return filepath.Dir(absPath), "file://" + absPath, nil
				} else {
					return "", "", err
				}
			} else {
				return "", "", err
			}
		}
	}
}

const maxOpenURLRequestBytes int64 = 16 * 1024

type OpenURLRequest struct {
	URL    string `json:"url"`
	Driver string `json:"driver,omitempty"`
}

type OpenURLResponse struct {
	ResID       string     `json:"res_id"`
	URL         string     `json:"url"`
	Name        string     `json:"name"`
	Kind        string     `json:"kind"`
	Size        int        `json:"size"`
	ContentType string     `json:"content_type"`
	Status      string     `json:"status"`
	Resources   []Resource `json:"resources"`
	Driver      string     `json:"driver"`
	Session     string     `json:"session"`
}

func filterCLIArgs(args []string) []string {
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.HasPrefix(arg, "-psn_") {
			// macOS LaunchServices passes a process serial number argument on Finder launch.
		} else {
			filtered = append(filtered, arg)
		}
	}
	return filtered
}

func isAllowedOpenURLDriver(driver string) bool {
	for _, allowed := range uploadDriverNames {
		if allowed == driver {
			return true
		} else {
			// Continue checking remaining byte-filter drivers.
		}
	}
	return false
}

func selectOpenURLDriver(requested string, detectedKind string) (string, error) {
	if requested == "" || requested == "auto_driver" {
		driver, expandable := driverForKind(detectedKind)
		if !expandable || driver == "url_driver" {
			return "text_driver", nil
		} else {
			return driver, nil
		}
	} else if !isAllowedOpenURLDriver(requested) {
		return "", fmt.Errorf("cannot open URL with unsupported driver %q: expected auto_driver or a byte-filter driver", requested)
	} else {
		return requested, nil
	}
}

func openURLHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required for /api/openURL", http.StatusMethodNotAllowed)
			return
		} else {
			var req OpenURLRequest
			decoder := json.NewDecoder(io.LimitReader(r.Body, maxOpenURLRequestBytes+1))
			if err := decoder.Decode(&req); err != nil {
				http.Error(w, fmt.Sprintf("cannot decode /api/openURL request: expected JSON with url and optional driver, got error: %v", err), http.StatusBadRequest)
				return
			} else {
				parsed, urlErr := urldriver.ValidateURL(req.URL)
				if urlErr != nil {
					http.Error(w, urlErr.Error(), http.StatusBadRequest)
					return
				} else {
					fetched, fetchErr := urldriver.FetchURL(parsed.String(), store.maxUploadBytes, urldriver.ResolveTimeout())
					if fetchErr != nil {
						if strings.Contains(fetchErr.Error(), "-byte limit") {
							http.Error(w, fetchErr.Error(), http.StatusRequestEntityTooLarge)
						} else {
							http.Error(w, fetchErr.Error(), http.StatusBadGateway)
						}
						return
					} else {
						driver, driverErr := selectOpenURLDriver(req.Driver, fetched.Kind)
						if driverErr != nil {
							http.Error(w, driverErr.Error(), http.StatusBadRequest)
							return
						} else {
							resID, putErr := store.putBlob(fetched.Body)
							if putErr != nil {
								http.Error(w, fmt.Sprintf("cannot retain fetched URL bytes: %v", putErr), http.StatusInsufficientStorage)
								return
							} else {
								resources, loadErr := store.loadDriverForBlob(driver, resID)
								if loadErr != nil {
									store.mu.Lock()
									store.removeBlobLocked(resID)
									store.mu.Unlock()
									http.Error(w, fmt.Sprintf("cannot load driver %s on URL %s: %v", driver, fetched.FinalURL, loadErr), http.StatusInternalServerError)
									return
								} else {
									if store.savedState != nil {
										_, _ = store.savedState.recordRecent(SavedEntryRequest{
											Label: fetched.FinalURL,
											URL:   fetched.FinalURL,
											Kind:  fetched.Kind,
										})
									} else {
										// No persistent saved-state store attached in unit tests.
									}
									w.Header().Set("Content-Type", "application/json")
									_ = json.NewEncoder(w).Encode(OpenURLResponse{
										ResID:       resID,
										URL:         fetched.FinalURL,
										Name:        fetched.DisplayName,
										Kind:        fetched.Kind,
										Size:        len(fetched.Body),
										ContentType: fetched.ContentType,
										Status:      fetched.Status,
										Resources:   resources,
										Driver:      driver,
										Session:     fmt.Sprintf("%s_%s", driver, resID),
									})
								}
							}
						}
					}
				}
			}
		}
	}
}
