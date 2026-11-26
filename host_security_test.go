package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoadHandlerRejectsDisallowedDrivers(t *testing.T) {
	store := NewStore()
	defer store.stopDrivers()

	resID, err := store.putBlob([]byte("/etc"))
	if err != nil {
		t.Fatalf("putBlob failed: %v", err)
	}

	handler := loadHandler(store)
	disallowed := []string{
		"directory_driver",
		"git_driver",
		"aws_driver",
		"github_driver",
		"kubernetes_driver",
		"url_driver",
		"root_driver",
		"system_profiler_driver",
		"",
		"nonexistent_driver",
	}
	for _, driver := range disallowed {
		body, _ := json.Marshal(map[string]string{"res_id": resID, "driver": driver})
		req := httptest.NewRequest(http.MethodPost, "/api/load", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("POST /api/load driver=%q status = %d, want 400 (body: %s)", driver, rec.Code, rec.Body.String())
		}
	}
}

func TestLoadChildHandlerEnforcesDriverAllowlist(t *testing.T) {
	store := NewStore()
	defer store.stopDrivers()
	store.mu.Lock()
	store.drivers["root_driver"] = &driverProc{}
	store.mu.Unlock()
	defer func() {
		store.mu.Lock()
		delete(store.drivers, "root_driver")
		store.mu.Unlock()
	}()

	textHandle := store.addHandle(ChildHandle{
		Name:       "notes.txt",
		ChildName:  "notes.txt",
		Kind:       "text",
		DriverName: "root_driver",
	})
	handler := loadChildHandler(store)

	disallowed := []string{
		"directory_driver",
		"git_driver",
		"aws_driver",
		"github_driver",
		"kubernetes_driver",
		"url_driver",
		"root_driver",
		"system_profiler_driver",
	}
	for _, driver := range disallowed {
		body, _ := json.Marshal(map[string]string{"handle": textHandle, "driver": driver})
		req := httptest.NewRequest(http.MethodPost, "/api/loadChild", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("POST /api/loadChild driver=%q status = %d, want 400 (body: %s)", driver, rec.Code, rec.Body.String())
		}
	}

	// Kind escalation without explicit driver must also be rejected.
	escalateBody, _ := json.Marshal(map[string]string{"handle": textHandle, "kind": "directory"})
	escalateReq := httptest.NewRequest(http.MethodPost, "/api/loadChild", bytes.NewReader(escalateBody))
	escalateRec := httptest.NewRecorder()
	handler.ServeHTTP(escalateRec, escalateReq)
	if escalateRec.Code != http.StatusBadRequest {
		t.Fatalf("POST /api/loadChild kind escalation status = %d, want 400 (body: %s)", escalateRec.Code, escalateRec.Body.String())
	}

	// Safe byte-filter drivers and the handle's own trusted kind/driver are allowed.
	if !isAllowedChildDriver("json_driver", "text") {
		t.Fatal("expected byte-filter driver json_driver to be allowed on text handle")
	}
	if !isAllowedChildDriver("directory_driver", "directory") {
		t.Fatal("expected directory_driver to be allowed on directory handle")
	}
	if isAllowedChildDriver("directory_driver", "text") {
		t.Fatal("expected directory_driver to be rejected on text handle")
	}
}

func TestAutoLoadHandlerRejectsKindEscalation(t *testing.T) {
	store := NewStore()
	defer store.stopDrivers()
	store.mu.Lock()
	store.drivers["root_driver"] = &driverProc{}
	store.mu.Unlock()
	defer func() {
		store.mu.Lock()
		delete(store.drivers, "root_driver")
		store.mu.Unlock()
	}()

	textHandle := store.addHandle(ChildHandle{
		Name:       "notes.txt",
		ChildName:  "notes.txt",
		Kind:       "text",
		DriverName: "root_driver",
	})
	handler := autoLoadHandler(store)

	body, _ := json.Marshal(map[string]string{"handle": textHandle, "kind": "directory"})
	req := httptest.NewRequest(http.MethodPost, "/api/autoLoad", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /api/autoLoad kind escalation status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestAPIJSONBodyLimits(t *testing.T) {
	store := NewStore()
	defer store.stopDrivers()

	oversized := `{"driver":"text_driver","res_id":"` + strings.Repeat("a", int(maxAPIJSONRequestBytes)+1024) + `"}`
	endpoints := []struct {
		path    string
		handler http.HandlerFunc
	}{
		{"/api/load", loadHandler(store)},
		{"/api/loadChild", loadChildHandler(store)},
		{"/api/autoLoad", autoLoadHandler(store)},
		{"/api/unload", unloadHandler(store)},
	}
	for _, ep := range endpoints {
		req := httptest.NewRequest(http.MethodPost, ep.path, strings.NewReader(oversized))
		rec := httptest.NewRecorder()
		ep.handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("POST %s oversized body status = %d, want 400", ep.path, rec.Code)
		}
	}
}

func TestNewHTTPServerSetsTimeouts(t *testing.T) {
	srv := newHTTPServer(http.NewServeMux())
	if srv.ReadHeaderTimeout <= 0 {
		t.Fatalf("ReadHeaderTimeout = %v, want > 0", srv.ReadHeaderTimeout)
	}
	if srv.IdleTimeout <= 0 {
		t.Fatalf("IdleTimeout = %v, want > 0", srv.IdleTimeout)
	}
}
