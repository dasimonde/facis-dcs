package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"digital-contracting-service/internal/auth/oid4vp/status"
	"digital-contracting-service/internal/auth/oid4vp/status/fetch"
	"digital-contracting-service/internal/auth/oid4vp/status/handler"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestW3CBitstring_Check_Ed25519Signature2020(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	fixturePath := filepath.Join(filepath.Dir(thisFile), "testdata", "w3c_bitstring_ed25519_signature_2020.json")
	raw, err := os.ReadFile(fixturePath)
	require.NoError(t, err)

	// Offline fixture is served from httptest; no external status-list host is required.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/ld+json")
		_, _ = w.Write(raw)
	}))
	t.Cleanup(srv.Close)

	trust := &status.TrustConfig{
		Issuers: map[string]status.TrustIssuerEntry{
			"did:key:z6Mkg165pEHaUPxkY4NxToor7suxzawEmdT1DEWq3e1Nr2VR": {
				JWKS: status.TrustJWKS{Keys: []map[string]any{
					{
						"kty": "OKP",
						"crv": "Ed25519",
						"x":   "FwMCVqKeYP_r4XVkmsXoH76spQd5enaETQAyJgUsecw",
					},
				}},
			},
		},
	}

	bitstringHandler := &handler.W3CBitstring{
		Fetcher: &fetch.Client{HTTPClient: &http.Client{Transport: rewriteHostTransport{base: srv.URL}}},
		Trust:   trust,
	}

	// The reference names the list by the URI the fixture identifies itself with,
	// which is what the handler binds against; rewriteHostTransport is what sends
	// the fetch to the httptest server instead.
	// The credential this list governs is issued by the same did:key that signed
	// the list — which is what makes the list its statement rather than some
	// other trusted issuer's (ADR-34).
	credential := status.VerifiedCredential{Claims: map[string]any{
		"iss": "did:key:z6Mkg165pEHaUPxkY4NxToor7suxzawEmdT1DEWq3e1Nr2VR",
	}}

	result, err := bitstringHandler.Check(context.Background(), credential, status.Reference{
		URI:       "http://localhost:7996/jGidIZFO8f6ey8wZ7Bn8",
		Index:     1,
		Purpose:   "revocation",
		Mechanism: status.MechanismW3CBitstring,
		EntryType: "BitstringStatusListEntry",
	})
	require.NoError(t, err)
	assert.Equal(t, status.StateValid, result.State)
}
