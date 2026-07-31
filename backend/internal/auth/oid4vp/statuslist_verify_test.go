package oid4vp

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"digital-contracting-service/internal/auth/oid4vp/status"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	_ = ConfigureStatusListVerification(nil, true)
	os.Exit(m.Run())
}

// The status-list verifier is built from the trust config already parsed for
// OID4VP, not from a second read of the same file. Only issuers carrying a
// bundled JWKS can sign a status list: an issuer whose key is resolved by x5c,
// did:web or ORCE has no key to check a list signature against, so it must be
// absent from the projection rather than present and unusable.
func TestConfigureStatusListVerificationTakesOnlyIssuersWithABundledKey(t *testing.T) {
	trustCfg := &TrustConfig{Issuers: map[string]TrustedIssuer{
		"did:web:example:issuer:bundled": {
			Mechanism: MechanismJWKS,
			JWKS:      json.RawMessage(`{"keys":[{"kty":"EC","crv":"P-256","x":"f83OJ3D2xF1Bg8vub9tLe1gHMzV76e8Tus9uPHvRVEU","y":"x_FEzRu9m36HLN_tue659LNpXW6pCyStikYjKIWI5a0","kid":"bundled"}]}`),
		},
		"did:web:example:issuer:x5c": {
			Mechanism: MechanismX5C,
		},
	}}

	require.NoError(t, ConfigureStatusListVerification(trustCfg, true))
	t.Cleanup(func() { _ = ConfigureStatusListVerification(nil, true) })

	projected, err := status.NewTrustConfig(map[string]json.RawMessage{
		"did:web:example:issuer:bundled": trustCfg.Issuers["did:web:example:issuer:bundled"].JWKS,
		"did:web:example:issuer:x5c":     trustCfg.Issuers["did:web:example:issuer:x5c"].JWKS,
	})
	require.NoError(t, err)
	assert.Contains(t, projected.Issuers, "did:web:example:issuer:bundled")
	assert.NotContains(t, projected.Issuers, "did:web:example:issuer:x5c")
}

func makeXFSCListBody(bitstring []byte) []byte {
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

func TestCheckStatusList_IETFStatusList_Active(t *testing.T) {
	bitstring := make([]byte, 125000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.EqualFold(strings.TrimSpace(r.Header.Get("Content-Type")), status.XFSCSignedContentType) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		assert.Equal(t, status.IETFStatusListAccept, r.Header.Get("Accept"))
		assert.Empty(t, r.Header.Get("Content-Type"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(makeXFSCListBody(bitstring))
	}))
	defer srv.Close()

	claims, err := json.Marshal(map[string]any{
		"status": map[string]any{
			"status_list": map[string]any{
				"uri": srv.URL,
				"idx": 62073,
			},
		},
	})
	require.NoError(t, err)
	require.NoError(t, checkStatusList(claims))
}

func TestCheckStatusList_IETFStatusList_Revoked(t *testing.T) {
	idx := uint32(3)
	bitstring := make([]byte, 16)
	bitstring[idx/8] |= 1 << (idx % 8)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.EqualFold(strings.TrimSpace(r.Header.Get("Content-Type")), status.XFSCSignedContentType) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		assert.Equal(t, status.IETFStatusListAccept, r.Header.Get("Accept"))
		assert.Empty(t, r.Header.Get("Content-Type"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(makeXFSCListBody(bitstring))
	}))
	defer srv.Close()

	claims, err := json.Marshal(map[string]any{
		"status": map[string]any{
			"status_list": map[string]any{
				"uri": srv.URL,
				"idx": idx,
			},
		},
	})
	require.NoError(t, err)

	err = checkStatusList(claims)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "revoked")
}
