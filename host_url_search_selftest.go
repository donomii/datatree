package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
)

func checkURLOpeningAndDriver(fixtureDir string) error {
	nestedZip, err := makeTestZip([]testZipEntry{
		{Name: "from-url.txt", Data: []byte("url zip payload")},
	})
	if err != nil {
		return fmt.Errorf("cannot build URL test zip: %v", err)
	} else {
		// Continue with local test HTTP server.
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/info.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"service":"datatree","remote":true}`))
		case "/bundle.zip":
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(nestedZip)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	// 1. CLI root URL support through root_driver -> url_driver -> json_driver.
	targetURL := server.URL + "/info.json"
	resolved, resolveErr := resolveRootDirectory([]string{targetURL})
	if resolveErr != nil || resolved != targetURL {
		return fmt.Errorf("resolveRootDirectory(%q) = (%q, %v), expected exact URL", targetURL, resolved, resolveErr)
	} else {
		// Root URL argument accepted.
	}

	urlStore := NewStore()
	defer urlStore.stopDrivers()
	urlStore.rootDirectory = resolved
	rootResources, rootErr := urlStore.loadRootDriver()
	if rootErr != nil {
		return fmt.Errorf("loadRootDriver for URL %q failed: %v", targetURL, rootErr)
	} else {
		// Root URL listing loaded.
	}
	urlHandle, handleErr := findResourceHandle(urlStore, rootResources, targetURL, "url")
	if handleErr != nil {
		return fmt.Errorf("root URL listing did not expose %q as kind url: %v", targetURL, handleErr)
	} else {
		// URL handle located.
	}
	urlChildren, driverErr := urlStore.loadDriverForHandle("url_driver", urlHandle)
	if driverErr != nil {
		return fmt.Errorf("url_driver failed for %q: %v", targetURL, driverErr)
	} else {
		// URL fetched via url_driver.
	}
	jsonChildHandle, childErr := findResourceHandle(urlStore, urlChildren, "info.json", "json")
	if childErr != nil {
		return fmt.Errorf("url_driver did not expose info.json child: %v", childErr)
	} else {
		// Fetched JSON child found.
	}
	jsonFields, jsonErr := urlStore.loadDriverForHandle("json_driver", jsonChildHandle)
	if jsonErr != nil {
		return fmt.Errorf("json_driver failed on fetched URL child: %v", jsonErr)
	} else if _, err := findResource(jsonFields, "service: datatree", "kv"); err != nil {
		return fmt.Errorf("fetched URL JSON missing service field: %v", err)
	} else {
		// Fetched URL JSON parsed through handle stack.
	}

	// 2. HTTP POST /api/openURL + GET /api/data?res_id=... for a ZIP URL.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/openURL", openURLHandler(urlStore))
	reqBody, _ := json.Marshal(OpenURLRequest{URL: server.URL + "/bundle.zip"})
	req := httptest.NewRequest(http.MethodPost, "/api/openURL", bytes.NewReader(reqBody))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		return fmt.Errorf("/api/openURL status = %d, body = %s", rec.Code, rec.Body.String())
	} else {
		var resp OpenURLResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			return fmt.Errorf("cannot decode /api/openURL response: %v", err)
		} else if resp.Driver != "zip_driver" || resp.ResID == "" {
			return fmt.Errorf("unexpected /api/openURL response: %+v", resp)
		} else if _, err := findResource(resp.Resources, "from-url.txt", "text"); err != nil {
			return fmt.Errorf("/api/openURL zip listing missing from-url.txt: %v", err)
		} else if raw := urlStore.serveBlobRange(resp.ResID, 0, -1); len(raw) != len(nestedZip) {
			return fmt.Errorf("stored URL blob size = %d, want %d", len(raw), len(nestedZip))
		} else {
			// /api/openURL fetched, retained, and expanded ZIP URL.
		}
	}

	// 3. Speedsearch across unopened folded directories and nested archives.
	searchRoot := filepath.Join(fixtureDir, "speedsearch-tree")
	nestedDir := filepath.Join(searchRoot, "folded-folder", "deep")
	if err := os.MkdirAll(nestedDir, 0700); err != nil {
		return fmt.Errorf("cannot create speedsearch fixture directory: %v", err)
	} else if err := os.WriteFile(filepath.Join(nestedDir, "folded-secret.txt"), []byte("secret"), 0600); err != nil {
		return fmt.Errorf("cannot write speedsearch fixture file: %v", err)
	} else if err := os.WriteFile(filepath.Join(searchRoot, "folded-folder", "remote-bundle.zip"), nestedZip, 0600); err != nil {
		return fmt.Errorf("cannot write speedsearch fixture zip: %v", err)
	} else {
		// Fixture tree created.
	}

	searchStore := NewStore()
	defer searchStore.stopDrivers()
	searchStore.rootDirectory = searchRoot
	if _, err := searchStore.loadRootDriver(); err != nil {
		return fmt.Errorf("speedsearch loadRootDriver failed: %v", err)
	} else {
		// Root directory loaded.
	}

	fileResp := searchStore.searchResources(context.Background(), SearchRequest{Query: "folded-secret"})
	if len(fileResp.Matches) != 1 || fileResp.Matches[0].Resource.Name != "folded-secret.txt" || fileResp.Matches[0].Resource.Handle == "" {
		return fmt.Errorf("speedsearch did not find folded-secret.txt in unopened directory: %+v", fileResp)
	} else {
		// Matched file inside unopened directory with materialized handle.
	}

	zipResp := searchStore.searchResources(context.Background(), SearchRequest{Query: "from-url text"})
	foundInZip := false
	for _, match := range zipResp.Matches {
		if match.Resource.Name == "from-url.txt" && match.Resource.Handle != "" && len(match.Path) >= 2 && strings.Contains(match.Path[1].Name, "remote-bundle.zip") {
			foundInZip = true
		} else {
			// Continue scanning matches.
		}
	}
	if !foundInZip {
		return fmt.Errorf("speedsearch did not find from-url.txt inside unopened nested zip: %+v", zipResp)
	} else {
		// Matched file inside unopened archive inside unopened directory.
	}

	// 4. s3://bucket/path/to/file tree-target resolution + AWS S3 hierarchical walk.
	_, s3RootURL, s3TargetErr := resolveRootTarget([]string{"s3://example-bucket/path/to/report.json"})
	if s3TargetErr != nil || s3RootURL != "s3://example-bucket/path/to/report.json" {
		return fmt.Errorf("resolveRootTarget(s3://example-bucket/path/to/report.json) = (%q, %v)", s3RootURL, s3TargetErr)
	} else {
		// s3:// root URL validated cleanly.
	}

	fakeAWSPath := filepath.Join(fixtureDir, "fake-s3-nav-aws")
	fakeAWSScript := `#!/bin/sh
set -eu
cmd="$*"
case "$cmd" in
  "configure list-profiles")
    printf 'default\n'
    ;;
  *"configure get region"*)
    printf 'us-east-1\n'
    ;;
  *"configure get "*)
    exit 0
    ;;
  *"ec2 describe-regions"*)
    printf '{"Regions":[{"RegionName":"us-east-1","OptInStatus":"opt-in-not-required"}]}\n'
    ;;
  *"s3api list-buckets"*)
    printf '{"Buckets":[{"Name":"example-bucket"}]}\n'
    ;;
  *"s3api list-objects-v2"*"--prefix=path/to/"*)
    printf '{"Contents":[{"Key":"path/to/report.json","Size":13}]}\n'
    ;;
  *"s3api list-objects-v2"*"--prefix=path/"*)
    printf '{"CommonPrefixes":[{"Prefix":"path/to/"}]}\n'
    ;;
  *"s3api list-objects-v2"*"--prefix="*)
    printf '{"CommonPrefixes":[{"Prefix":"path/"}]}\n'
    ;;
  *"s3 cp s3://example-bucket/path/to/report.json -"*)
    printf '{"ok":"s3nav"}'
    ;;
  *)
    printf 'unexpected fake aws args: %s\n' "$cmd" >&2
    exit 1
    ;;
esac
`
	if err := os.WriteFile(fakeAWSPath, []byte(fakeAWSScript), 0700); err != nil {
		return fmt.Errorf("cannot write fake s3-nav aws script: %v", err)
	} else {
		prevAWS, hadPrevAWS := os.LookupEnv("DATATREE_AWS")
		if err := os.Setenv("DATATREE_AWS", fakeAWSPath); err != nil {
			return fmt.Errorf("cannot set DATATREE_AWS for s3 URL self-test: %v", err)
		} else {
			defer func() {
				if hadPrevAWS {
					_ = os.Setenv("DATATREE_AWS", prevAWS)
				} else {
					_ = os.Unsetenv("DATATREE_AWS")
				}
			}()
		}

		s3Store := NewStore()
		defer s3Store.stopDrivers()
		rootAWSResources, err := s3Store.loadRootDriver()
		if err != nil {
			return fmt.Errorf("loadRootDriver for s3:// test failed: %v", err)
		} else {
			awsHandle, err := findResourceHandle(s3Store, rootAWSResources, "aws", "aws")
			if err != nil {
				return fmt.Errorf("root listing missing virtual aws handle: %v", err)
			} else {
				awsSteps := []string{
					"default",
					"[regions and resources]",
					"us-east-1 (opt-in-not-required)",
					"[service catalog]",
					"S3 buckets",
					"example-bucket",
					"path/",
					"to/",
				}
				currentChildren, stepErr := s3Store.loadDriverForHandle("aws_driver", awsHandle)
				if stepErr != nil {
					return fmt.Errorf("aws_driver root expansion failed: %v", stepErr)
				} else {
					for _, stepName := range awsSteps {
						stepHandle, findErr := findResourceHandle(s3Store, currentChildren, stepName, "aws")
						if findErr != nil {
							return fmt.Errorf("s3 URL tree walk could not find %q: %v", stepName, findErr)
						} else {
							currentChildren, stepErr = s3Store.loadDriverForHandle("aws_driver", stepHandle)
							if stepErr != nil {
								return fmt.Errorf("s3 URL tree walk could not expand %q: %v", stepName, stepErr)
							} else {
								// Expanded step cleanly.
							}
						}
					}
					reportHandle, reportErr := findResourceHandle(s3Store, currentChildren, "report.json", "json")
					if reportErr != nil {
						return fmt.Errorf("s3 URL tree walk missing final object report.json: %v", reportErr)
					} else {
						jsonRows, jsonLoadErr := s3Store.loadDriverForHandle("json_driver", reportHandle)
						if jsonLoadErr != nil {
							return fmt.Errorf("json_driver failed on s3://example-bucket/path/to/report.json: %v", jsonLoadErr)
						} else if _, kvErr := findResource(jsonRows, "ok: s3nav", "kv"); kvErr != nil {
							return fmt.Errorf("s3 object JSON missing expected ok field: %v", kvErr)
						} else {
							// Full s3://bucket/path/to/file path walked and inspected cleanly.
						}
					}
				}
			}
		}
	}

	// 5. Bookmarks and recents HTTP + persistence check inside fixture directory.
	bmDir := filepath.Join(fixtureDir, "bookmarks-config")
	savedStore, savedErr := openSavedStateStoreAt(bmDir)
	if savedErr != nil {
		return fmt.Errorf("openSavedStateStoreAt failed: %v", savedErr)
	} else {
		bmMux := http.NewServeMux()
		mountBookmarks(bmMux, savedStore)
		if _, _, err := savedStore.addBookmark(SavedEntryRequest{Label: "report", URL: "s3://example-bucket/path/to/report.json", Kind: "json"}); err != nil {
			return fmt.Errorf("addBookmark in self-test failed: %v", err)
		} else if _, err := savedStore.recordRecent(SavedEntryRequest{Label: "report", URL: "s3://example-bucket/path/to/report.json", Kind: "json"}); err != nil {
			return fmt.Errorf("recordRecent in self-test failed: %v", err)
		} else {
			bmGetReq := httptest.NewRequest(http.MethodGet, "/api/bookmarks", nil)
			bmGetW := httptest.NewRecorder()
			bmMux.ServeHTTP(bmGetW, bmGetReq)
			var gotBM SavedEntriesResponse
			if bmGetW.Code != http.StatusOK || json.Unmarshal(bmGetW.Body.Bytes(), &gotBM) != nil || len(gotBM.Entries) != 1 || gotBM.Entries[0].URL != "s3://example-bucket/path/to/report.json" {
				return fmt.Errorf("/api/bookmarks self-test mismatch: code=%d body=%s", bmGetW.Code, bmGetW.Body.String())
			} else {
				recGetReq := httptest.NewRequest(http.MethodGet, "/api/recents", nil)
				recGetW := httptest.NewRecorder()
				bmMux.ServeHTTP(recGetW, recGetReq)
				var gotRec SavedEntriesResponse
				if recGetW.Code != http.StatusOK || json.Unmarshal(recGetW.Body.Bytes(), &gotRec) != nil || len(gotRec.Entries) != 1 || gotRec.Entries[0].URL != "s3://example-bucket/path/to/report.json" {
					return fmt.Errorf("/api/recents self-test mismatch: code=%d body=%s", recGetW.Code, recGetW.Body.String())
				} else {
					// Bookmarks and recents endpoints verified.
				}
			}
		}
	}

	return nil
}
