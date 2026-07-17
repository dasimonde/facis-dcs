package command

import (
	"net/url"
	"strings"
	"testing"
)

func TestBuildCeremonyWalletURI(t *testing.T) {
	got := buildCeremonyWalletURI("http://localhost:5173/api", "abc-123")
	if !strings.HasPrefix(got, "openid4vp://?") {
		t.Fatalf("unexpected scheme: %s", got)
	}
	q, err := url.ParseQuery(strings.TrimPrefix(got, "openid4vp://?"))
	if err != nil {
		t.Fatalf("parse query: %v", err)
	}
	if q.Get("client_id") != ceremonyAudience {
		t.Fatalf("client_id=%q", q.Get("client_id"))
	}
	if q.Get("request_uri_method") != "post" {
		t.Fatalf("request_uri_method=%q", q.Get("request_uri_method"))
	}
	if q.Get("nonce") != "" {
		t.Fatalf("nonce must not appear in deep link, got %q", q.Get("nonce"))
	}
	wantURI := "http://localhost:5173/api/signature/presentation/request/abc-123"
	if q.Get("request_uri") != wantURI {
		t.Fatalf("request_uri=%q want %q", q.Get("request_uri"), wantURI)
	}
}
