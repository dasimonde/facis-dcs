package status_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"digital-contracting-service/internal/auth/oid4vp/status"
	"digital-contracting-service/internal/auth/oid4vp/status/fetch"
	"digital-contracting-service/internal/auth/oid4vp/status/handler"

	"github.com/golang-jwt/jwt/v5"
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

const statusListIssuer = "http://status.example"

// xfscSignedList serves a status list in the XFSC shape: the unsigned
// {tenantId, listId, list} envelope on the plain probe, and — only for the
// request that asks for statuslist+jwt — the same bits inside a signed token.
//
// The probe body decides nothing; it exists because its media type is what
// routes the reference to the XFSC handler. The verdict always comes from the
// signed token, so a test built on this fixture cannot pass on a list nobody
// vouched for.
type xfscSignedList struct {
	key  *ecdsa.PrivateKey
	bits []byte
}

func newXFSCSignedList(t *testing.T, bits []byte) *xfscSignedList {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	return &xfscSignedList{key: key, bits: bits}
}

// signedToken is what the issuer serves: ES256 over typ statuslist+jwt, iss the
// issuer's identifier, sub the list's own URL, bits gzip-deflated under
// status_list.lst.
func (l *xfscSignedList) signedToken(t *testing.T, listURI string) string {
	t.Helper()

	token := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"iss": statusListIssuer,
		"sub": listURI,
		"iat": time.Now().Add(-time.Minute).Unix(),
		"status_list": map[string]any{
			"bits": 1,
			"lst":  base64.RawURLEncoding.EncodeToString(gzipBytes(l.bits)),
		},
	})
	token.Header["typ"] = "statuslist+jwt"
	signed, err := token.SignedString(l.key)
	require.NoError(t, err)
	return signed
}

func (l *xfscSignedList) unsignedEnvelope() []byte {
	body, _ := json.Marshal(map[string]any{
		"tenantId": "default",
		"listId":   1,
		"list":     base64.RawStdEncoding.EncodeToString(gzipBytes(l.bits)),
	})
	return body
}

// trustedJWKS publishes the list's own signing key. Passing a different key
// here is what a test uses to prove the signature is checked rather than
// assumed.
func (l *xfscSignedList) trustedJWKS(pub *ecdsa.PublicKey) *status.TrustConfig {
	return &status.TrustConfig{Issuers: map[string]status.TrustIssuerEntry{
		statusListIssuer: {JWKS: status.TrustJWKS{Keys: []map[string]any{{
			"kty": "EC",
			"crv": "P-256",
			"x":   base64.RawURLEncoding.EncodeToString(pub.X.FillBytes(make([]byte, 32))),
			"y":   base64.RawURLEncoding.EncodeToString(pub.Y.FillBytes(make([]byte, 32))),
		}}}},
	}}
}

func gzipBytes(raw []byte) []byte {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, _ = w.Write(raw)
	_ = w.Close()
	return buf.Bytes()
}

// newVerifierForSignedList wires a verifier the way a deployment does, against
// a server that answers both the routing probe and the signed fetch.
func newVerifierForSignedList(t *testing.T, list *xfscSignedList, trust *status.TrustConfig) *status.Verifier {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.EqualFold(strings.TrimSpace(r.Header.Get("Content-Type")), status.XFSCSignedContentType) {
			w.Header().Set("Content-Type", "application/statuslist+jwt")
			_, _ = w.Write([]byte(list.signedToken(t, statusListURI)))
			return
		}
		assert.Equal(t, status.IETFStatusListAccept, r.Header.Get("Accept"))
		assert.Empty(t, r.Header.Get("Content-Type"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(list.unsignedEnvelope())
	}))
	t.Cleanup(srv.Close)

	fetcher := &fetch.Client{HTTPClient: &http.Client{Transport: localTransport{base: srv.URL}}}
	verifier := handler.NewVerifier(trust, handler.Options{})
	verifier.Fetcher = fetcher
	verifier.Handlers[status.MechanismXFSC] = &handler.XFSC{Fetcher: fetcher, Trust: trust}
	return verifier
}

const statusListURI = statusListIssuer + "/list"

func statusListCredential(index uint64) status.VerifiedCredential {
	return status.VerifiedCredential{
		Format: "sd-jwt",
		Claims: map[string]any{
			"status": map[string]any{
				"status_list": map[string]any{
					"uri": statusListURI,
					"idx": index,
				},
			},
		},
	}
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
	list := newXFSCSignedList(t, make([]byte, 125000))
	verifier := newVerifierForSignedList(t, list, list.trustedJWKS(&list.key.PublicKey))

	result, err := verifier.VerifyStatus(context.Background(), statusListCredential(62073))
	require.NoError(t, err)
	require.True(t, result.Accepted)
	assert.Equal(t, status.MechanismXFSC, result.StatusResults[0].Mechanism)
	assert.Equal(t, status.StateValid, result.StatusResults[0].State)
}

func TestVerifier_IETFStatusList_Revoked(t *testing.T) {
	const idx uint64 = 3
	bitstring := make([]byte, 16)
	bitstring[idx/8] |= 1 << (idx % 8)

	list := newXFSCSignedList(t, bitstring)
	verifier := newVerifierForSignedList(t, list, list.trustedJWKS(&list.key.PublicKey))

	result, err := verifier.VerifyStatus(context.Background(), statusListCredential(idx))
	require.NoError(t, err)
	require.False(t, result.Accepted)
	assert.Equal(t, status.StateInvalid, result.StatusResults[0].State)
}

// TestVerifier_IETFStatusList_RefusesAListSignedByAnUntrustedKey pins that the
// two tests above pass on the signature and not merely on the bits: the same
// server, the same envelope, one key the trust config does not name.
func TestVerifier_IETFStatusList_RefusesAListSignedByAnUntrustedKey(t *testing.T) {
	list := newXFSCSignedList(t, make([]byte, 16))
	other, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	verifier := newVerifierForSignedList(t, list, list.trustedJWKS(&other.PublicKey))

	_, err = verifier.VerifyStatus(context.Background(), statusListCredential(0))
	require.ErrorIs(t, err, status.ErrStatusSignature)
}
