package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"digital-contracting-service/internal/base/event"
)

const readinessEndpointPath = "/readyz"

// livenessEndpointPath answers 200 while the process serves HTTP, during
// initialization too (DCS-NFR-PER-03), and its outbox jobs make progress. The
// liveness probe uses it, so Kubernetes restarts a process that stops serving
// or whose outbox job hangs (DCS-NFR-SF-01).
const livenessEndpointPath = "/livez"

func readinessResponse(status int, message string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(status)
		_, _ = fmt.Fprintln(w, message)
	}
}

func bootstrapHTTPHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle(livenessEndpointPath, readinessResponse(http.StatusOK, "alive"))
	mux.Handle("/", readinessResponse(
		http.StatusServiceUnavailable,
		"starting up: initialization has not completed",
	))
	return mux
}

func mountReadinessEndpoint(mux *http.ServeMux) {
	mux.Handle(readinessEndpointPath, readinessResponse(http.StatusOK, "ready"))
	mux.Handle(livenessEndpointPath, livenessWith(event.StuckJobs))
}

func livenessWith(stuckJobs func(time.Time) []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if stuck := stuckJobs(time.Now()); len(stuck) > 0 {
			readinessResponse(http.StatusServiceUnavailable, "stuck: "+strings.Join(stuck, ", "))(w, r)
			return
		}
		readinessResponse(http.StatusOK, "alive")(w, r)
	}
}
