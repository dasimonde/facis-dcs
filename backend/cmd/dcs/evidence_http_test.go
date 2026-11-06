package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"digital-contracting-service/internal/base/conf"

	"github.com/stretchr/testify/require"
)

func TestEvidenceBodyLimit(t *testing.T) {
	t.Setenv("DCS_EVIDENCE_MAX_FILE_BYTES", "2")
	t.Setenv("DCS_EVIDENCE_MAX_TEXT_BYTES", "2")
	for _, path := range []string{"/contract/store", "/api/contract/store"} {
		called := false
		handler := evidenceBodyLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true; w.WriteHeader(http.StatusOK) }))
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(strings.Repeat("x", int(conf.EvidenceRequestLimit())+1)))
		request.ContentLength = -1
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		require.Equal(t, http.StatusRequestEntityTooLarge, response.Code)
		require.False(t, called)
	}
}
