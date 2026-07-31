package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBootstrapReadinessIsUnavailable(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, readinessEndpointPath, nil)
	rec := httptest.NewRecorder()

	bootstrapHTTPHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	if !strings.Contains(rec.Body.String(), "initialization has not completed") {
		t.Fatalf("body = %q, want initialization diagnostic", rec.Body.String())
	}
}

func TestFinalReadinessIsUnauthenticatedAndReady(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mountReadinessEndpoint(mux)
	req := httptest.NewRequest(http.MethodGet, readinessEndpointPath, nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != "ready\n" {
		t.Fatalf("body = %q, want ready", got)
	}
}
