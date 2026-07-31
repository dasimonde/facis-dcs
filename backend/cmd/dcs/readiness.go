package main

import (
	"fmt"
	"net/http"
)

const readinessEndpointPath = "/readyz"

func readinessResponse(status int, message string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(status)
		_, _ = fmt.Fprintln(w, message)
	}
}

func bootstrapHTTPHandler() http.Handler {
	return readinessResponse(
		http.StatusServiceUnavailable,
		"starting up: initialization has not completed",
	)
}

func mountReadinessEndpoint(mux *http.ServeMux) {
	mux.Handle(readinessEndpointPath, readinessResponse(http.StatusOK, "ready"))
}
