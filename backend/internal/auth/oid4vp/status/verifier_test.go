package status_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"digital-contracting-service/internal/auth/oid4vp/status"
	"digital-contracting-service/internal/auth/oid4vp/status/fetch"
	"digital-contracting-service/internal/auth/oid4vp/status/handler"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifier_IETFStatusList_NoGlobalProbe(t *testing.T) {
	var requestCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		assert.Equal(t, status.IETFStatusListAccept, r.Header.Get("Accept"))
		assert.Empty(t, r.Header.Get("Content-Type"))

		w.Header().Set("Content-Type", "application/statuslist+jwt")
		_, _ = w.Write([]byte(`eyJhbGciOiJFUzI1NiJ9.eyJzdWIiOiJ1cmkifQ.sig`))
	}))
	t.Cleanup(srv.Close)

	trust := &status.TrustConfig{
		Issuers: map[string]status.TrustIssuerEntry{
			"did:web:example:issuer": {
				JWKS: status.TrustJWKS{Keys: []map[string]any{
					{
						"kty": "EC",
						"crv": "P-256",
						"x":   "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
						"y":   "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
					},
				}},
			},
		},
	}

	fetcher := &fetch.Client{HTTPClient: &http.Client{Transport: localTransport{base: srv.URL}}}
	verifier := handler.NewVerifier(trust, handler.Options{})
	verifier.Fetcher = fetcher

	_, err := verifier.VerifyStatus(context.Background(), status.VerifiedCredential{
		Format: "sd-jwt",
		Claims: map[string]any{
			"status": map[string]any{
				"status_list": map[string]any{
					"uri": srv.URL,
					"idx": 0,
				},
			},
		},
	})
	require.Error(t, err)
	assert.Equal(t, int32(1), requestCount.Load(), "standard IETF verification must not probe before handler fetch")
}

func makeXFSCUnsignedListBody(bitstring []byte) []byte {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, _ = w.Write(bitstring)
	_ = w.Close()
	body, _ := json.Marshal(map[string]any{
		"tenantId": "default",
		"listId":   1,
		"list":     base64.RawStdEncoding.EncodeToString(buf.Bytes()),
	})
	return body
}

func newVerifierWithInlineList(t *testing.T, listBody []byte) *status.Verifier {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.EqualFold(strings.TrimSpace(r.Header.Get("Content-Type")), status.XFSCSignedContentType) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		assert.Equal(t, status.IETFStatusListAccept, r.Header.Get("Accept"))
		assert.Empty(t, r.Header.Get("Content-Type"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(listBody)
	}))
	t.Cleanup(srv.Close)

	fetcher := &fetch.Client{HTTPClient: &http.Client{Transport: localTransport{base: srv.URL}}}
	verifier := handler.NewVerifier(nil, handler.Options{})
	verifier.Fetcher = fetcher
	verifier.Handlers[status.MechanismXFSC] = &handler.XFSC{Fetcher: fetcher}
	return verifier
}

type localTransport struct {
	base string
}

func (t localTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.URL.Scheme = "http"
	cloned.URL.Host = strings.TrimPrefix(strings.TrimPrefix(t.base, "https://"), "http://")
	return http.DefaultTransport.RoundTrip(cloned)
}

func TestVerifier_IETFStatusList_Active(t *testing.T) {
	bitstring := make([]byte, 125000)
	verifier := newVerifierWithInlineList(t, makeXFSCUnsignedListBody(bitstring))

	result, err := verifier.VerifyStatus(context.Background(), status.VerifiedCredential{
		Format: "sd-jwt",
		Claims: map[string]any{
			"status": map[string]any{
				"status_list": map[string]any{
					"uri": "http://status.example/list",
					"idx": 62073,
				},
			},
		},
	})
	require.NoError(t, err)
	require.True(t, result.Accepted)
	assert.Equal(t, status.MechanismXFSC, result.StatusResults[0].Mechanism)
	assert.Equal(t, status.StateValid, result.StatusResults[0].State)
}

func TestVerifier_IETFStatusList_Revoked(t *testing.T) {
	const idx uint64 = 3
	bitstring := make([]byte, 16)
	bitstring[idx/8] |= 1 << (idx % 8)

	verifier := newVerifierWithInlineList(t, makeXFSCUnsignedListBody(bitstring))

	result, err := verifier.VerifyStatus(context.Background(), status.VerifiedCredential{
		Format: "sd-jwt",
		Claims: map[string]any{
			"status": map[string]any{
				"status_list": map[string]any{
					"uri": "http://status.example/list",
					"idx": idx,
				},
			},
		},
	})
	require.NoError(t, err)
	require.False(t, result.Accepted)
	assert.Equal(t, status.StateInvalid, result.StatusResults[0].State)
}
