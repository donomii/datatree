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

func TestBookmarksAndRecentsPersistenceAndHTTP(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("DATATREE_CONFIG_DIR", configDir)

	store, err := openSavedStateStore()
	if err != nil {
		t.Fatalf("openSavedStateStore failed: %v", err)
	} else {
		// Store initialized in isolated config directory.
	}

	mux := http.NewServeMux()
	mountBookmarks(mux, store)

	// 1. Add S3 bookmark via POST /api/bookmarks.
	addBody, _ := json.Marshal(SavedEntryRequest{
		Label: "prod report",
		URL:   "s3://example-bucket/path/to/report.json",
		Kind:  "json",
	})
	addReq := httptest.NewRequest(http.MethodPost, "/api/bookmarks", bytes.NewReader(addBody))
	addRec := httptest.NewRecorder()
	mux.ServeHTTP(addRec, addReq)
	if addRec.Code != http.StatusOK {
		t.Fatalf("POST /api/bookmarks status = %d, body = %s", addRec.Code, addRec.Body.String())
	} else {
		// Bookmark saved.
	}

	var saveResp SaveBookmarkResponse
	if decErr := json.Unmarshal(addRec.Body.Bytes(), &saveResp); decErr != nil {
		t.Fatalf("cannot decode SaveBookmarkResponse: %v", decErr)
	} else if len(saveResp.Entries) != 1 || saveResp.Entry.ID == "" || saveResp.Entry.URL != "s3://example-bucket/path/to/report.json" {
		t.Fatalf("unexpected SaveBookmarkResponse: %+v", saveResp)
	} else {
		// Returned bookmark has generated ID and normalized URL.
	}

	// Verify file written cleanly on disk and survives store reopen.
	if _, statErr := os.Stat(filepath.Join(configDir, bookmarksFileName)); statErr != nil {
		t.Fatalf("bookmarks.json was not written on disk: %v", statErr)
	} else {
		// Bookmarks file persisted.
	}
	reopened, reopenErr := openSavedStateStore()
	if reopenErr != nil {
		t.Fatalf("reopen saved store failed: %v", reopenErr)
	} else if listed, listErr := reopened.listBookmarks(); listErr != nil || len(listed) != 1 || listed[0].Label != "prod report" {
		t.Fatalf("reopened bookmarks mismatch: %+v err=%v", listed, listErr)
	} else {
		// Bookmarks survived restart.
	}

	// 2. Record recents with deduplication/bubbling.
	for _, url := range []string{"s3://bucket/a.txt", "file:///tmp/b.zip", "s3://bucket/a.txt"} {
		recBody, _ := json.Marshal(SavedEntryRequest{URL: url, Kind: "file"})
		recReq := httptest.NewRequest(http.MethodPost, "/api/recents", bytes.NewReader(recBody))
		recW := httptest.NewRecorder()
		mux.ServeHTTP(recW, recReq)
		if recW.Code != http.StatusOK {
			t.Fatalf("POST /api/recents failed: status=%d body=%s", recW.Code, recW.Body.String())
		} else {
			// Recent entry recorded.
		}
	}

	getRecReq := httptest.NewRequest(http.MethodGet, "/api/recents", nil)
	getRecW := httptest.NewRecorder()
	mux.ServeHTTP(getRecW, getRecReq)
	var recResp SavedEntriesResponse
	if decErr := json.Unmarshal(getRecW.Body.Bytes(), &recResp); decErr != nil {
		t.Fatalf("cannot decode /api/recents response: %v", decErr)
	} else if len(recResp.Entries) != 2 || recResp.Entries[0].URL != "s3://bucket/a.txt" || recResp.Entries[1].URL != "file:///tmp/b.zip" {
		t.Fatalf("recents deduplication/order mismatch: %+v", recResp.Entries)
	} else {
		// Recent item bubbled to top cleanly.
	}

	// 3. Delete bookmark via DELETE /api/bookmarks?id=...
	delReq := httptest.NewRequest(http.MethodDelete, "/api/bookmarks?id="+saveResp.Entry.ID, nil)
	delW := httptest.NewRecorder()
	mux.ServeHTTP(delW, delReq)
	var delResp DeleteBookmarkResponse
	if decErr := json.Unmarshal(delW.Body.Bytes(), &delResp); decErr != nil {
		t.Fatalf("cannot decode DELETE /api/bookmarks response: %v", decErr)
	} else if !delResp.Removed || len(delResp.Entries) != 0 {
		t.Fatalf("DELETE /api/bookmarks did not remove bookmark: %+v", delResp)
	} else {
		// Bookmark removed cleanly.
	}

	// 4. Clear recents via DELETE /api/recents.
	clrReq := httptest.NewRequest(http.MethodDelete, "/api/recents", nil)
	clrW := httptest.NewRecorder()
	mux.ServeHTTP(clrW, clrReq)
	if clrW.Code != http.StatusOK {
		t.Fatalf("DELETE /api/recents status = %d", clrW.Code)
	} else if after, err := store.listRecents(); err != nil || len(after) != 0 {
		t.Fatalf("recents not cleared: %+v err=%v", after, err)
	} else {
		// Recents cleared.
	}

	// 5. GET /static/bookmarks.js served cleanly.
	jsReq := httptest.NewRequest(http.MethodGet, "/static/bookmarks.js", nil)
	jsW := httptest.NewRecorder()
	mux.ServeHTTP(jsW, jsReq)
	if jsW.Code != http.StatusOK || !strings.Contains(jsW.Body.String(), "initBookmarksAndRecents") {
		t.Fatalf("GET /static/bookmarks.js failed: code=%d", jsW.Code)
	} else {
		// Bookmarks client module served.
	}
}
