package handler_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"digital-contracting-service/internal/auth/oid4vp/status"
	"digital-contracting-service/internal/auth/oid4vp/status/envelope"
	"digital-contracting-service/internal/auth/oid4vp/status/fetch"
	"digital-contracting-service/internal/auth/oid4vp/status/handler"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	bindingListIssuer  = "did:web:lists.example:issuer"
	bindingOtherIssuer = "did:web:other.example:issuer"
	bindingListURI     = "https://lists.example/status/1"
)

// An all-zero bitstring: entry 1 is not revoked, so a list that is accepted
// reports valid and one that is refused cannot be mistaken for it.
const bindingEncodedList = "uH4sIAAAAAAAAA-3BMQEAAADCoPVPbQsvoAAAAAAAAAAAAAAAAP4GcwM92tQwAAA"

func bindingJWK(pub *ecdsa.PublicKey) map[string]any {
	return map[string]any{
		"kty": "EC",
		"crv": "P-256",
		"x":   base64.RawURLEncoding.EncodeToString(pub.X.FillBytes(make([]byte, 32))),
		"y":   base64.RawURLEncoding.EncodeToString(pub.Y.FillBytes(make([]byte, 32))),
	}
}

// signedStatusList returns a BitstringStatusListCredential signed by signingKey
// while NAMING namedIssuer as its issuer, so the two can be pulled apart.
func signedStatusList(t *testing.T, signingKey *ecdsa.PrivateKey, signingIssuer, namedIssuer, listID string) []byte {
	t.Helper()
	document := map[string]any{
		"@context": []any{"https://www.w3.org/ns/credentials/v2"},
		"id":       listID,
		"type":     []any{"VerifiableCredential", "BitstringStatusListCredential"},
		"issuer":   namedIssuer,
		"credentialSubject": map[string]any{
			"id":            listID + "#list",
			"type":          "BitstringStatusList",
			"statusPurpose": "revocation",
			"encodedList":   bindingEncodedList,
		},
	}
	signed, err := envelope.SignDataIntegrityCredential(document, envelope.ECDSASigner{
		PrivateKey:           signingKey,
		VerificationMethodID: signingIssuer + "#key-1",
	}, "")
	require.NoError(t, err)
	raw, err := json.Marshal(signed)
	require.NoError(t, err)
	return raw
}

func bindingHandler(t *testing.T, body []byte, trusted map[string]*ecdsa.PublicKey) *handler.W3CBitstring {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/ld+json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	issuers := map[string]status.TrustIssuerEntry{}
	for issuer, pub := range trusted {
		issuers[issuer] = status.TrustIssuerEntry{JWKS: status.TrustJWKS{Keys: []map[string]any{bindingJWK(pub)}}}
	}
	return &handler.W3CBitstring{
		Fetcher: &fetch.Client{HTTPClient: &http.Client{Transport: rewriteHostTransport{base: srv.URL}}},
		Trust:   &status.TrustConfig{Issuers: issuers},
	}
}

// The credential these tests govern is issued by bindingListIssuer, which is
// what lets its list be believed at all: a list is the statement of the issuer
// that issued the credential (ADR-34). The binding under test here is the other
// half — that the SIGNER is the issuer the list names.
func checkList(h *handler.W3CBitstring, uri string) (status.Result, error) {
	credential := status.VerifiedCredential{Claims: map[string]any{"iss": bindingListIssuer}}
	return h.Check(context.Background(), credential, status.Reference{
		URI:       uri,
		Index:     1,
		Purpose:   "revocation",
		Mechanism: status.MechanismW3CBitstring,
		EntryType: "BitstringStatusListEntry",
	})
}

func TestW3CBitstring_AcceptsAListItsOwnIssuerSigned(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	body := signedStatusList(t, key, bindingListIssuer, bindingListIssuer, bindingListURI)
	h := bindingHandler(t, body, map[string]*ecdsa.PublicKey{bindingListIssuer: &key.PublicKey})

	result, err := checkList(h, bindingListURI)
	require.NoError(t, err)
	assert.Equal(t, status.StateValid, result.State)
}

// The finding this closes: every issuer in the trust configuration is resolvable
// by name, so a signature alone only says SOME trusted issuer signed the list.
// Without the bind, an issuer trusted for its own credentials could sign another
// issuer's revocation list and un-revoke a credential it has no authority over.
func TestW3CBitstring_RefusesAListSignedByAnotherTrustedIssuer(t *testing.T) {
	attacker, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	owner, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	// Signed by the attacker's key and naming the attacker's verification method,
	// but claiming to be the list of bindingListIssuer.
	body := signedStatusList(t, attacker, bindingOtherIssuer, bindingListIssuer, bindingListURI)
	h := bindingHandler(t, body, map[string]*ecdsa.PublicKey{
		bindingOtherIssuer: &attacker.PublicKey,
		bindingListIssuer:  &owner.PublicKey,
	})

	_, err = checkList(h, bindingListURI)
	require.ErrorIs(t, err, status.ErrStatusListIssuerMismatch)
}

// A genuine list for one URI must not answer for another: the entry index is only
// meaningful in the list the credential pointed at.
func TestW3CBitstring_RefusesAListThatIsNotTheOneReferenced(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	body := signedStatusList(t, key, bindingListIssuer, bindingListIssuer, "https://lists.example/status/99")
	h := bindingHandler(t, body, map[string]*ecdsa.PublicKey{bindingListIssuer: &key.PublicKey})

	_, err = checkList(h, bindingListURI)
	require.ErrorIs(t, err, status.ErrStatusURIMismatch)
}
