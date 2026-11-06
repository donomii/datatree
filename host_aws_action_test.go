package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAWSActionHandlerRejectsCrossOriginRequest(t *testing.T) {
	store := NewStore()
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8081/api/aws/action", strings.NewReader(`{"handle":"missing"}`))
	request.Header.Set("Origin", "https://attacker.example")
	response := httptest.NewRecorder()
	awsActionHandler(store).ServeHTTP(response, request)
	switch {
	case response.Code != http.StatusForbidden:
		t.Fatalf("cross-origin AWS action status mismatch: got %d expected %d with body %q", response.Code, http.StatusForbidden, response.Body.String())
	default:
	}
}

func TestAWSActionHandlerAcceptsSameOriginBeforeHandleValidation(t *testing.T) {
	store := NewStore()
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8081/api/aws/action", strings.NewReader(`{"handle":"missing"}`))
	request.Header.Set("Origin", "http://127.0.0.1:8081")
	response := httptest.NewRecorder()
	awsActionHandler(store).ServeHTTP(response, request)
	switch {
	case response.Code != http.StatusBadRequest:
		t.Fatalf("same-origin AWS action status mismatch: got %d expected %d with body %q", response.Code, http.StatusBadRequest, response.Body.String())
	case !strings.Contains(response.Body.String(), `handle "missing" is unknown`):
		t.Fatalf("same-origin AWS action error mismatch: got %q", response.Body.String())
	default:
	}
}
