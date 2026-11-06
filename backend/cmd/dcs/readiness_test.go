package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

// DCS-NFR-SF-01: an outbox job that hangs makes the liveness probe fail, so
// Kubernetes restarts the process and another run delivers the outbox.
func TestLivenessFailsWhileAnOutboxJobIsStuck(t *testing.T) {
	stuck := func(time.Time) []string { return []string{"outbox-anchoring (running since 2026-10-31T10:00:00Z)"} }
	rec := httptest.NewRecorder()
	livenessWith(stuck)(rec, httptest.NewRequest(http.MethodGet, livenessEndpointPath, nil))
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "outbox-anchoring") {
		t.Fatalf("liveness with a stuck job = %d %q, want 503 naming the job", rec.Code, rec.Body.String())
	}
}

// DCS-NFR-PER-03: the liveness probe sees a serving process as alive during
// initialization and after it.
func TestLivenessDuringAndAfterInitialization(t *testing.T) {
	t.Parallel()

	final := http.NewServeMux()
	mountReadinessEndpoint(final)
	for name, h := range map[string]http.Handler{"bootstrap": bootstrapHTTPHandler(), "final": final} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, livenessEndpointPath, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s %s status = %d, want %d", name, livenessEndpointPath, rec.Code, http.StatusOK)
		}
	}
}
