package main

import (
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//go:embed web/bookmarks.js
var bookmarksScript []byte

const (
	bookmarksFileName       = "bookmarks.json"
	recentsFileName         = "recents.json"
	maxSavedEntryBytes      = 32 * 1024
	maxSavedBookmarks       = 200
	maxSavedRecents         = 50
	savedStoreFormatVersion = 1
)

type SavedPathSegment struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type SavedEntry struct {
	ID        string             `json:"id"`
	Label     string             `json:"label"`
	URL       string             `json:"url,omitempty"`
	Path      []SavedPathSegment `json:"path,omitempty"`
	Kind      string             `json:"kind,omitempty"`
	UpdatedAt string             `json:"updated_at"`
}

type savedFilePayload struct {
	Version int          `json:"version"`
	Entries []SavedEntry `json:"entries"`
}

type SavedEntriesResponse struct {
	Entries []SavedEntry `json:"entries"`
}

type SaveBookmarkResponse struct {
	Entry   SavedEntry   `json:"entry"`
	Entries []SavedEntry `json:"entries"`
}

type DeleteBookmarkResponse struct {
	Removed bool         `json:"removed"`
	Entries []SavedEntry `json:"entries"`
}

type SavedEntryRequest struct {
	Label string             `json:"label"`
	URL   string             `json:"url,omitempty"`
	Path  []SavedPathSegment `json:"path,omitempty"`
	Kind  string             `json:"kind,omitempty"`
}

type DeleteBookmarkRequest struct {
	ID   string             `json:"id,omitempty"`
	URL  string             `json:"url,omitempty"`
	Path []SavedPathSegment `json:"path,omitempty"`
}

type SavedStateStore struct {
	dir  string
	gate chan struct{}
}

func resolveDatatreeConfigDir() (string, error) {
	override := strings.TrimSpace(os.Getenv("DATATREE_CONFIG_DIR"))
	if override != "" {
		return override, nil
	} else {
		base, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("cannot locate Datatree configuration directory: %v", err)
		} else {
			return filepath.Join(base, "datatree"), nil
		}
	}
}

func openSavedStateStore() (*SavedStateStore, error) {
	dir, err := resolveDatatreeConfigDir()
	if err != nil {
		return nil, err
	} else {
		return openSavedStateStoreAt(dir)
	}
}

func openSavedStateStoreAt(dir string) (*SavedStateStore, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("cannot create Datatree state directory %q: %v", dir, err)
	} else {
		return &SavedStateStore{
			dir:  dir,
			gate: make(chan struct{}, 1),
		}, nil
	}
}

func newSavedID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("cannot generate saved-entry identifier: %v", err)
	} else {
		return "bm-" + hex.EncodeToString(buf), nil
	}
}

func savedEntryKey(url string, path []SavedPathSegment) string {
	trimmedURL := strings.TrimSpace(url)
	if trimmedURL != "" {
		return "url:" + trimmedURL
	} else {
		raw, err := json.Marshal(path)
		if err != nil {
			return "path:invalid"
		} else {
			return "path:" + string(raw)
		}
	}
}

func normalizeSavedEntry(req SavedEntryRequest) (SavedEntry, error) {
	trimmedURL := strings.TrimSpace(req.URL)
	label := strings.TrimSpace(req.Label)
	cleanPath := make([]SavedPathSegment, 0, len(req.Path))
	for _, seg := range req.Path {
		if strings.TrimSpace(seg.Name) == "" {
			continue
		} else {
			cleanPath = append(cleanPath, SavedPathSegment{Name: seg.Name, Kind: strings.TrimSpace(seg.Kind)})
		}
	}
	if trimmedURL == "" && len(cleanPath) == 0 {
		return SavedEntry{}, fmt.Errorf("cannot save entry: expected a non-empty url or tree path")
	} else {
		if label == "" {
			if trimmedURL != "" {
				label = trimmedURL
			} else {
				label = cleanPath[len(cleanPath)-1].Name
			}
		} else {
			// Preserve caller-supplied label.
		}
		kind := strings.TrimSpace(req.Kind)
		if kind == "" && len(cleanPath) > 0 {
			kind = cleanPath[len(cleanPath)-1].Kind
		} else {
			// Retain explicit kind or leave empty for URL-only entries.
		}
		return SavedEntry{
			Label:     label,
			URL:       trimmedURL,
			Path:      cleanPath,
			Kind:      kind,
			UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		}, nil
	}
}

func (s *SavedStateStore) readEntriesLocked(filename string) ([]SavedEntry, error) {
	fullPath := filepath.Join(s.dir, filename)
	raw, err := os.ReadFile(fullPath)
	if os.IsNotExist(err) {
		return []SavedEntry{}, nil
	} else if err != nil {
		return nil, fmt.Errorf("cannot read %q: %v", fullPath, err)
	} else {
		var payload savedFilePayload
		if decErr := json.Unmarshal(raw, &payload); decErr != nil {
			return nil, fmt.Errorf("cannot parse %q: expected JSON saved-entries payload, got error: %v", fullPath, decErr)
		} else if payload.Entries == nil {
			return []SavedEntry{}, nil
		} else {
			return payload.Entries, nil
		}
	}
}

func (s *SavedStateStore) writeEntriesLocked(filename string, entries []SavedEntry) error {
	if err := os.MkdirAll(s.dir, 0700); err != nil {
		return fmt.Errorf("cannot create directory %q for %s: %v", s.dir, filename, err)
	} else {
		payload := savedFilePayload{Version: savedStoreFormatVersion, Entries: entries}
		encoded, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return fmt.Errorf("cannot encode %s JSON: %v", filename, err)
		} else {
			target := filepath.Join(s.dir, filename)
			tempPath := target + ".tmp"
			if writeErr := os.WriteFile(tempPath, append(encoded, '\n'), 0600); writeErr != nil {
				return fmt.Errorf("cannot write temporary state file %q: %v", tempPath, writeErr)
			} else if renameErr := os.Rename(tempPath, target); renameErr != nil {
				return fmt.Errorf("cannot commit state file %q: %v", target, renameErr)
			} else {
				return nil
			}
		}
	}
}

func (s *SavedStateStore) listBookmarks() ([]SavedEntry, error) {
	s.gate <- struct{}{}
	defer func() { <-s.gate }()
	return s.readEntriesLocked(bookmarksFileName)
}

func (s *SavedStateStore) addBookmark(req SavedEntryRequest) (SavedEntry, []SavedEntry, error) {
	entry, normErr := normalizeSavedEntry(req)
	if normErr != nil {
		return SavedEntry{}, nil, normErr
	} else {
		s.gate <- struct{}{}
		defer func() { <-s.gate }()
		entries, err := s.readEntriesLocked(bookmarksFileName)
		if err != nil {
			return SavedEntry{}, nil, err
		} else {
			targetKey := savedEntryKey(entry.URL, entry.Path)
			filtered := make([]SavedEntry, 0, len(entries)+1)
			preservedID := ""
			for _, existing := range entries {
				if savedEntryKey(existing.URL, existing.Path) == targetKey {
					preservedID = existing.ID
				} else {
					filtered = append(filtered, existing)
				}
			}
			if preservedID != "" {
				entry.ID = preservedID
			} else {
				id, idErr := newSavedID()
				if idErr != nil {
					return SavedEntry{}, nil, idErr
				} else {
					entry.ID = id
				}
			}
			updated := append([]SavedEntry{entry}, filtered...)
			if len(updated) > maxSavedBookmarks {
				updated = updated[:maxSavedBookmarks]
			} else {
				// Within bookmark capacity bound.
			}
			if writeErr := s.writeEntriesLocked(bookmarksFileName, updated); writeErr != nil {
				return SavedEntry{}, nil, writeErr
			} else {
				return entry, updated, nil
			}
		}
	}
}

func (s *SavedStateStore) removeBookmark(req DeleteBookmarkRequest) ([]SavedEntry, bool, error) {
	targetID := strings.TrimSpace(req.ID)
	targetKey := ""
	if targetID == "" {
		targetKey = savedEntryKey(req.URL, req.Path)
	} else {
		// Removal will match explicit bookmark identifier.
	}
	if targetID == "" && targetKey == "path:null" && strings.TrimSpace(req.URL) == "" && len(req.Path) == 0 {
		return nil, false, fmt.Errorf("cannot delete bookmark: expected an id, url, or path")
	} else {
		s.gate <- struct{}{}
		defer func() { <-s.gate }()
		entries, err := s.readEntriesLocked(bookmarksFileName)
		if err != nil {
			return nil, false, err
		} else {
			kept := make([]SavedEntry, 0, len(entries))
			removed := false
			for _, existing := range entries {
				matchID := targetID != "" && existing.ID == targetID
				matchKey := targetKey != "" && savedEntryKey(existing.URL, existing.Path) == targetKey
				if matchID || matchKey {
					removed = true
				} else {
					kept = append(kept, existing)
				}
			}
			if removed {
				if writeErr := s.writeEntriesLocked(bookmarksFileName, kept); writeErr != nil {
					return nil, false, writeErr
				} else {
					return kept, true, nil
				}
			} else {
				return entries, false, nil
			}
		}
	}
}

func (s *SavedStateStore) listRecents() ([]SavedEntry, error) {
	s.gate <- struct{}{}
	defer func() { <-s.gate }()
	return s.readEntriesLocked(recentsFileName)
}

func (s *SavedStateStore) recordRecent(req SavedEntryRequest) ([]SavedEntry, error) {
	entry, normErr := normalizeSavedEntry(req)
	if normErr != nil {
		return nil, normErr
	} else {
		s.gate <- struct{}{}
		defer func() { <-s.gate }()
		entries, err := s.readEntriesLocked(recentsFileName)
		if err != nil {
			return nil, err
		} else {
			id, idErr := newSavedID()
			if idErr != nil {
				return nil, idErr
			} else {
				entry.ID = id
			}
			targetKey := savedEntryKey(entry.URL, entry.Path)
			filtered := make([]SavedEntry, 0, len(entries)+1)
			filtered = append(filtered, entry)
			for _, existing := range entries {
				if savedEntryKey(existing.URL, existing.Path) == targetKey {
					continue
				} else {
					filtered = append(filtered, existing)
				}
			}
			if len(filtered) > maxSavedRecents {
				filtered = filtered[:maxSavedRecents]
			} else {
				// Within max recents capacity.
			}
			if writeErr := s.writeEntriesLocked(recentsFileName, filtered); writeErr != nil {
				return nil, writeErr
			} else {
				return filtered, nil
			}
		}
	}
}

func (s *SavedStateStore) clearRecents() error {
	s.gate <- struct{}{}
	defer func() { <-s.gate }()
	return s.writeEntriesLocked(recentsFileName, []SavedEntry{})
}

func mountBookmarks(mux *http.ServeMux, saved *SavedStateStore) {
	mux.HandleFunc("GET /static/bookmarks.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = w.Write(bookmarksScript)
	})

	mux.HandleFunc("/api/bookmarks", func(w http.ResponseWriter, r *http.Request) {
		if saved == nil {
			http.Error(w, "bookmark store is unavailable", http.StatusInternalServerError)
			return
		} else if r.Method == http.MethodGet {
			entries, err := saved.listBookmarks()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			} else {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(SavedEntriesResponse{Entries: entries})
			}
		} else if r.Method == http.MethodPost {
			var req SavedEntryRequest
			dec := json.NewDecoder(io.LimitReader(r.Body, maxSavedEntryBytes))
			if err := dec.Decode(&req); err != nil {
				http.Error(w, fmt.Sprintf("cannot decode /api/bookmarks request: expected JSON with label and url or path, got error: %v", err), http.StatusBadRequest)
				return
			} else {
				entry, entries, err := saved.addBookmark(req)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				} else {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(SaveBookmarkResponse{Entry: entry, Entries: entries})
				}
			}
		} else if r.Method == http.MethodDelete {
			id := strings.TrimSpace(r.URL.Query().Get("id"))
			var req DeleteBookmarkRequest
			if id != "" {
				req.ID = id
			} else if r.Body != nil {
				_ = json.NewDecoder(io.LimitReader(r.Body, maxSavedEntryBytes)).Decode(&req)
			} else {
				// Empty DELETE body handled by validation below.
			}
			entries, removed, err := saved.removeBookmark(req)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			} else {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(DeleteBookmarkResponse{Removed: removed, Entries: entries})
			}
		} else {
			http.Error(w, "GET, POST, or DELETE required for /api/bookmarks", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("POST /api/bookmarks/delete", func(w http.ResponseWriter, r *http.Request) {
		if saved == nil {
			http.Error(w, "bookmark store is unavailable", http.StatusInternalServerError)
			return
		} else {
			var req DeleteBookmarkRequest
			if err := json.NewDecoder(io.LimitReader(r.Body, maxSavedEntryBytes)).Decode(&req); err != nil {
				http.Error(w, fmt.Sprintf("cannot decode /api/bookmarks/delete request: %v", err), http.StatusBadRequest)
				return
			} else {
				entries, removed, err := saved.removeBookmark(req)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				} else {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(DeleteBookmarkResponse{Removed: removed, Entries: entries})
				}
			}
		}
	})

	mux.HandleFunc("/api/recents", func(w http.ResponseWriter, r *http.Request) {
		if saved == nil {
			http.Error(w, "recents store is unavailable", http.StatusInternalServerError)
			return
		} else if r.Method == http.MethodGet {
			entries, err := saved.listRecents()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			} else {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(SavedEntriesResponse{Entries: entries})
			}
		} else if r.Method == http.MethodPost {
			var req SavedEntryRequest
			if err := json.NewDecoder(io.LimitReader(r.Body, maxSavedEntryBytes)).Decode(&req); err != nil {
				http.Error(w, fmt.Sprintf("cannot decode /api/recents request: %v", err), http.StatusBadRequest)
				return
			} else {
				entries, err := saved.recordRecent(req)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				} else {
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(SavedEntriesResponse{Entries: entries})
				}
			}
		} else if r.Method == http.MethodDelete {
			if err := saved.clearRecents(); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			} else {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(SavedEntriesResponse{Entries: []SavedEntry{}})
			}
		} else {
			http.Error(w, "GET, POST, or DELETE required for /api/recents", http.StatusMethodNotAllowed)
		}
	})
}
