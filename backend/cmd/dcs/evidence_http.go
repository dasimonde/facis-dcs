package main

import (
	"bytes"
	"io"
	"net/http"
	"strings"

	"digital-contracting-service/internal/base/conf"
)

// Bound the encoded JSON before Goa allocates a decoded upload, including chunked requests.
func evidenceBodyLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/contract/store") {
			body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, conf.EvidenceRequestLimit()))
			if err != nil {
				http.Error(w, "evidence request exceeds the configured limit", http.StatusRequestEntityTooLarge)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
		}
		next.ServeHTTP(w, r)
	})
}
