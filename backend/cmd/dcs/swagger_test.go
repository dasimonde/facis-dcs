package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	goahttp "goa.design/goa/v3/http"
)

func TestSwaggerUIRefusesFraming(t *testing.T) {
	mux := goahttp.NewMuxer()
	mountSwaggerUI(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/swagger", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /swagger = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("X-Frame-Options = %q, want DENY", got)
	}
	if got := rec.Header().Get("Content-Security-Policy"); got != "frame-ancestors 'none'" {
		t.Errorf("Content-Security-Policy = %q, want frame-ancestors 'none'", got)
	}
}
