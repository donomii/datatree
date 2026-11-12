package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"datahumper/drivers_go/driverlib"
)

//go:embed web/speedsearch.js
var speedsearchScript []byte

const defaultSearchMaxDepth = 6
const maxAllowedSearchDepth = 12
const defaultSearchMaxResults = 100
const maxAllowedSearchResults = 500
const defaultSearchMaxNodes = 3000
const maxAllowedSearchNodes = 10000
const maxSearchContainers = 40
const searchTimeout = 3 * time.Second
const maxSearchRequestBytes int64 = 64 * 1024

type SearchRequest struct {
	Query        string   `json:"query"`
	Handles      []string `json:"handles,omitempty"`
	ExtraHandles []string `json:"extra_handles,omitempty"`
	MaxDepth     int      `json:"max_depth,omitempty"`
	MaxResults   int      `json:"max_results,omitempty"`
	MaxNodes     int      `json:"max_nodes,omitempty"`
}

type SearchPathNode struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Handle string `json:"handle,omitempty"`
}

type SearchMatch struct {
	Path     []SearchPathNode `json:"path"`
	Resource Resource         `json:"resource"`
}

type SearchResponse struct {
	Query     string        `json:"query"`
	Matches   []SearchMatch `json:"matches"`
	Scanned   int           `json:"scanned"`
	Truncated bool          `json:"truncated"`
}

type searchDirStep struct {
	name   string
	kind   string
	path   string
	handle string
}

type searchQueueItem struct {
	resource Resource
	path     []SearchPathNode
	dirChain []searchDirStep
	depth    int
}

type searchSession struct {
	store       *Store
	tokens      []string
	maxDepth    int
	maxResults  int
	maxNodes    int
	scanned     int
	containers  int
	truncated   bool
	matches     []SearchMatch
	seenKeys    map[string]bool
	dirChildren map[string][]Resource
}

func mountSearch(mux *http.ServeMux, store *Store) {
	mux.HandleFunc("GET /static/speedsearch.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = w.Write(speedsearchScript)
	})
	mux.HandleFunc("/api/search", searchHandler(store))
}

func searchHandler(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST required for /api/search", http.StatusMethodNotAllowed)
			return
		} else {
			var req SearchRequest
			decoder := json.NewDecoder(io.LimitReader(r.Body, maxSearchRequestBytes+1))
			if err := decoder.Decode(&req); err != nil {
				http.Error(w, fmt.Sprintf("cannot decode /api/search request: expected JSON with query, got error: %v", err), http.StatusBadRequest)
				return
			} else {
				resp := store.searchResources(r.Context(), req)
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(resp)
			}
		}
	}
}

func clampPositive(value int, fallback int, maxVal int) int {
	if value <= 0 {
		return fallback
	} else if value > maxVal {
		return maxVal
	} else {
		return value
	}
}

func matchesSearchTokens(name string, kind string, tokens []string) bool {
	if len(tokens) == 0 {
		return false
	} else {
		haystack := strings.ToLower(name + " " + kind)
		for _, token := range tokens {
			if !strings.Contains(haystack, token) {
				return false
			} else {
				// Token matched; check remaining tokens.
			}
		}
		return true
	}
}

func isSearchSkippedRootVirtual(store *Store, r Resource) bool {
	if store.rootDirectory == "" && r.Kind == "directory" && r.Name == "/" {
		// In location-browser mode, avoid walking the entire OS root unless explicitly opened.
		return true
	} else {
		switch r.Kind {
		case "kubernetes", "github", "aws", "hardware", "usb", "url":
			return true
		default:
			return false
		}
	}
}

func isSearchableContainerKind(kind string) bool {
	switch kind {
	case "archive", "zip", "tar", "gzip", "bzip2", "xz", "zstd", "zlib", "deflate",
		"json", "ndjson", "yaml", "toml", "xml", "ini", "properties", "dotenv",
		"email", "mbox", "csv", "tsv", "html", "pdf", "text", "diff",
		"sqlite", "bundle", "riff", "mp4", "asn1", "jwt", "pem", "urlencoded",
		"base64", "hex", "jsonstring", "k8s-pod", "k8s-workload", "k8s-service",
		"k8s-configmap", "k8s-node":
		return true
	default:
		return false
	}
}

func (s *searchSession) recordMatch(path []SearchPathNode, res Resource) {
	if len(s.matches) >= s.maxResults {
		s.truncated = true
		return
	} else {
		keyParts := make([]string, 0, len(path)+2)
		for _, seg := range path {
			keyParts = append(keyParts, seg.Kind+":"+seg.Name)
		}
		keyParts = append(keyParts, res.Kind+":"+res.Name)
		dedupKey := strings.Join(keyParts, "\x00")
		if s.seenKeys[dedupKey] {
			return
		} else {
			s.seenKeys[dedupKey] = true
			copiedPath := append([]SearchPathNode(nil), path...)
			s.matches = append(s.matches, SearchMatch{Path: copiedPath, Resource: res})
		}
	}
}

func (s *searchSession) loadDirChildrenCached(handleID string) ([]Resource, error) {
	if cached, ok := s.dirChildren[handleID]; ok {
		return cached, nil
	} else {
		h, err := s.store.resolveHandle(handleID)
		if err != nil {
			return nil, err
		} else {
			children, loadErr := s.store.loadDriverForHandle("directory_driver", h)
			if loadErr != nil {
				return nil, loadErr
			} else {
				s.dirChildren[handleID] = children
				return children, nil
			}
		}
	}
}

func findNamedResource(resources []Resource, name string, kind string) (Resource, bool) {
	for _, r := range resources {
		if r.Name == name && r.Kind == kind {
			return r, true
		} else {
			// Continue searching for exact name and kind match.
		}
	}
	for _, r := range resources {
		if r.Name == name {
			return r, true
		} else {
			// Fallback to name match if git detection changed kind.
		}
	}
	return Resource{}, false
}

func (s *searchSession) materializeDirChain(chain []searchDirStep) ([]SearchPathNode, string, error) {
	if len(chain) == 0 {
		return nil, "", fmt.Errorf("cannot materialize empty directory chain")
	} else {
		pathNodes := make([]SearchPathNode, len(chain))
		for i := range chain {
			if chain[i].handle == "" && i > 0 {
				parentChildren, err := s.loadDirChildrenCached(chain[i-1].handle)
				if err != nil {
					return nil, "", err
				} else {
					matched, ok := findNamedResource(parentChildren, chain[i].name, chain[i].kind)
					if !ok || matched.Handle == "" {
						return nil, "", fmt.Errorf("cannot locate directory handle for %q", chain[i].name)
					} else {
						chain[i].handle = matched.Handle
					}
				}
			} else {
				// Root of chain or previously materialized step already holds a handle.
			}
			pathNodes[i] = SearchPathNode{
				Name:   chain[i].name,
				Kind:   chain[i].kind,
				Handle: chain[i].handle,
			}
		}
		return pathNodes, chain[len(chain)-1].handle, nil
	}
}

func (s *searchSession) isFilesystemDirectoryHandle(handleID string) (string, bool) {
	h, err := s.store.resolveHandle(handleID)
	if err != nil {
		return "", false
	} else if h.DriverName != "root_driver" && !strings.HasPrefix(h.DriverName, "directory_driver_") {
		return "", false
	} else {
		rawPath, rangeErr := s.store.serveChildRange(h, 0, -1)
		if rangeErr != nil {
			return "", false
		} else {
			dirPath := string(rawPath)
			info, statErr := os.Stat(dirPath)
			if statErr != nil || !info.IsDir() {
				return "", false
			} else {
				return dirPath, true
			}
		}
	}
}

func (s *searchSession) expandDirectoryNode(ctx context.Context, item searchQueueItem, queue *[]searchQueueItem) {
	dirPath := item.dirChain[len(item.dirChain)-1].path
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return
	} else {
		for _, entry := range entries {
			if ctx.Err() != nil || s.scanned >= s.maxNodes || len(s.matches) >= s.maxResults {
				s.truncated = true
				return
			} else {
				name := entry.Name()
				if strings.HasPrefix(name, ".") {
					continue
				} else {
					info, infoErr := entry.Info()
					if infoErr != nil {
						continue
					} else {
						s.scanned++
						fullPath := filepath.Join(dirPath, name)
						if info.IsDir() {
							kind := "directory"
							if driverlib.HasGitMarker(fullPath) {
								kind = "git"
							} else {
								// Non-git directories retain kind "directory".
							}
							childRes := Resource{Name: name, Kind: kind, Size: 0}
							if matchesSearchTokens(name, kind, s.tokens) {
								pathNodes, parentHandle, matErr := s.materializeDirChain(item.dirChain)
								if matErr == nil {
									children, loadErr := s.loadDirChildrenCached(parentHandle)
									if loadErr == nil {
										if realRes, ok := findNamedResource(children, name, kind); ok {
											childRes = realRes
										} else {
											// Keep synthesized metadata if driver omitted entry.
										}
									} else {
										// Keep childRes without handle if load failed.
									}
									s.recordMatch(pathNodes, childRes)
								} else {
									// Skip unmaterializable entry.
								}
							} else {
								// Entry does not match query tokens.
							}
							if (kind == "directory" || kind == "git") && item.depth+1 < s.maxDepth {
								nextChain := append([]searchDirStep(nil), item.dirChain...)
								nextChain = append(nextChain, searchDirStep{name: name, kind: kind, path: fullPath})
								*queue = append(*queue, searchQueueItem{
									resource: childRes,
									dirChain: nextChain,
									depth:    item.depth + 1,
								})
							} else {
								// Depth bound reached or git directory not walked raw.
							}
						} else {
							kind := driverlib.KindForName(name)
							isMatch := matchesSearchTokens(name, kind, s.tokens)
							needsHandle := isMatch || (isSearchableContainerKind(kind) && item.depth+1 < s.maxDepth && s.containers < maxSearchContainers)
							if !needsHandle {
								continue
							} else {
								pathNodes, parentHandle, matErr := s.materializeDirChain(item.dirChain)
								if matErr != nil {
									continue
								} else {
									children, loadErr := s.loadDirChildrenCached(parentHandle)
									if loadErr != nil {
										continue
									} else {
										realRes, ok := findNamedResource(children, name, kind)
										if !ok {
											continue
										} else {
											if isMatch {
												s.recordMatch(pathNodes, realRes)
											} else {
												// Container file queued for inner search.
											}
											if isSearchableContainerKind(realRes.Kind) && realRes.Handle != "" && item.depth+1 < s.maxDepth && s.containers < maxSearchContainers {
												nextPath := append([]SearchPathNode(nil), pathNodes...)
												nextPath = append(nextPath, SearchPathNode{Name: realRes.Name, Kind: realRes.Kind, Handle: realRes.Handle})
												*queue = append(*queue, searchQueueItem{
													resource: realRes,
													path:     nextPath,
													depth:    item.depth + 1,
												})
											} else {
												// Leaf file or container budget reached.
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
}

func (s *searchSession) expandContainerNode(ctx context.Context, item searchQueueItem, queue *[]searchQueueItem) {
	if item.resource.Handle == "" || s.containers >= maxSearchContainers {
		if s.containers >= maxSearchContainers {
			s.truncated = true
		} else {
			// Missing handle cannot be expanded.
		}
		return
	} else {
		driver, expandable := driverForKind(item.resource.Kind)
		if !expandable || driver == "" {
			return
		} else {
			h, err := s.store.resolveHandle(item.resource.Handle)
			if err != nil {
				return
			} else {
				s.containers++
				children, loadErr := s.store.loadDriverForHandle(driver, h)
				if loadErr != nil {
					return
				} else {
					for _, child := range children {
						if ctx.Err() != nil || s.scanned >= s.maxNodes || len(s.matches) >= s.maxResults {
							s.truncated = true
							return
						} else if child.Kind == "raw" && child.Name == "[source bytes]" {
							continue
						} else {
							s.scanned++
							if matchesSearchTokens(child.Name, child.Kind, s.tokens) {
								s.recordMatch(item.path, child)
							} else {
								// Non-matching child may still have matching descendants.
							}
							if item.depth+1 < s.maxDepth && child.Handle != "" && isSearchableContainerKind(child.Kind) {
								nextPath := append([]SearchPathNode(nil), item.path...)
								nextPath = append(nextPath, SearchPathNode{Name: child.Name, Kind: child.Kind, Handle: child.Handle})
								*queue = append(*queue, searchQueueItem{
									resource: child,
									path:     nextPath,
									depth:    item.depth + 1,
								})
							} else {
								// Stop descending at leaf nodes or depth limit.
							}
						}
					}
				}
			}
		}
	}
}

func (s *Store) searchResources(ctx context.Context, req SearchRequest) SearchResponse {
	tokens := strings.Fields(strings.ToLower(req.Query))
	resp := SearchResponse{
		Query:   req.Query,
		Matches: []SearchMatch{},
	}
	if len(tokens) == 0 {
		return resp
	} else {
		ctx, cancel := context.WithTimeout(ctx, searchTimeout)
		defer cancel()

		session := &searchSession{
			store:       s,
			tokens:      tokens,
			maxDepth:    clampPositive(req.MaxDepth, defaultSearchMaxDepth, maxAllowedSearchDepth),
			maxResults:  clampPositive(req.MaxResults, defaultSearchMaxResults, maxAllowedSearchResults),
			maxNodes:    clampPositive(req.MaxNodes, defaultSearchMaxNodes, maxAllowedSearchNodes),
			matches:     []SearchMatch{},
			seenKeys:    map[string]bool{},
			dirChildren: map[string][]Resource{},
		}

		var queue []searchQueueItem
		enqueueHandle := func(hid string) {
			h, err := s.resolveHandle(hid)
			if err != nil {
				return
			} else {
				r := Resource{Name: h.Name, Kind: h.Kind, Size: h.Size, Handle: h.Hid}
				if dirPath, ok := session.isFilesystemDirectoryHandle(h.Hid); ok {
					queue = append(queue, searchQueueItem{
						resource: r,
						dirChain: []searchDirStep{{name: r.Name, kind: r.Kind, path: dirPath, handle: h.Hid}},
						depth:    0,
					})
				} else if isSearchableContainerKind(r.Kind) {
					queue = append(queue, searchQueueItem{
						resource: r,
						path:     []SearchPathNode{{Name: r.Name, Kind: r.Kind, Handle: h.Hid}},
						depth:    0,
					})
				} else {
					// Non-searchable handle skipped.
				}
			}
		}
		if len(req.Handles) > 0 {
			for _, hid := range req.Handles {
				enqueueHandle(hid)
			}
		} else {
			s.mu.RLock()
			roots := append([]Resource(nil), s.rootResources...)
			s.mu.RUnlock()
			for _, r := range roots {
				if isSearchSkippedRootVirtual(s, r) {
					continue
				} else {
					session.scanned++
					if matchesSearchTokens(r.Name, r.Kind, tokens) {
						session.recordMatch(nil, r)
					} else {
						// Root item did not match directly; descend if searchable.
					}
					if r.Handle == "" {
						continue
					} else if r.Kind == "directory" || (r.Kind == "git" && r.Name != ".") {
						if dirPath, ok := session.isFilesystemDirectoryHandle(r.Handle); ok {
							queue = append(queue, searchQueueItem{
								resource: r,
								dirChain: []searchDirStep{{name: r.Name, kind: r.Kind, path: dirPath, handle: r.Handle}},
								depth:    0,
							})
						} else {
							// Unreachable directory path skipped.
						}
					} else if isSearchableContainerKind(r.Kind) {
						queue = append(queue, searchQueueItem{
							resource: r,
							path:     []SearchPathNode{{Name: r.Name, Kind: r.Kind, Handle: r.Handle}},
							depth:    0,
						})
					} else {
						// Leaf root resource already checked above.
					}
				}
			}
			for _, hid := range req.ExtraHandles {
				enqueueHandle(hid)
			}
		}

		for len(queue) > 0 {
			if ctx.Err() != nil || session.scanned >= session.maxNodes || len(session.matches) >= session.maxResults {
				session.truncated = true
				break
			} else {
				current := queue[0]
				queue = queue[1:]
				if len(current.dirChain) > 0 {
					session.expandDirectoryNode(ctx, current, &queue)
				} else {
					session.expandContainerNode(ctx, current, &queue)
				}
			}
		}

		resp.Matches = session.matches
		resp.Scanned = session.scanned
		resp.Truncated = session.truncated
		return resp
	}
}
