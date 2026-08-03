package identity

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func testJWK(t *testing.T, key *ecdsa.PrivateKey, certDER []byte) map[string]any {
	t.Helper()
	jwk := map[string]any{
		"kty": "EC",
		"crv": "P-256",
		"x":   base64.RawURLEncoding.EncodeToString(key.X.FillBytes(make([]byte, 32))),
		"y":   base64.RawURLEncoding.EncodeToString(key.Y.FillBytes(make([]byte, 32))),
	}
	if certDER != nil {
		jwk["x5c"] = []string{base64.StdEncoding.EncodeToString(certDER)}
	}
	return jwk
}

func testCertificate(t *testing.T, host string, key *ecdsa.PrivateKey) []byte {
	t.Helper()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: host},
		DNSNames:     []string{host},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return der
}

// parseDocument goes through the same JSON path a resolved peer document takes.
func parseDocument(t *testing.T, doc map[string]any) *DIDDocument {
	t.Helper()
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	var parsed DIDDocument
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &parsed.didContent); err != nil {
		t.Fatal(err)
	}
	return &parsed
}

func loadDocument(t *testing.T, doc map[string]any, signer *ecdsa.PrivateKey) (*DIDDocument, error) {
	t.Helper()
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "did.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return NewDIDDocument(path, signer)
}

func TestResolveMethodID(t *testing.T) {
	const docID = "did:web:peer.example"
	for _, tc := range []struct {
		name, in, want string
		wantErr        bool
	}{
		{name: "relative fragment resolves against the document", in: "#key-1", want: docID + "#key-1"},
		{name: "absolute id of this document passes through", in: docID + "#key-1", want: docID + "#key-1"},
		{name: "another document's key is refused", in: "did:web:elsewhere.example#key-1", wantErr: true},
		{name: "nothing named is refused", in: "  ", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveMethodID(docID, tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected %q to be refused, got %q", tc.in, got)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("ResolveMethodID(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
			}
		})
	}
}

// A relationship names its keys by relative DID URL, embeds one inline, and the
// verificationMethod order says nothing — all conformant, none of it resolvable
// by position or by this deployment's own key labels.
func TestMethodForResolvesRelativeAndEmbeddedEntries(t *testing.T) {
	referenced, embedded, ecdh := testKey(t), testKey(t), testKey(t)
	doc := parseDocument(t, map[string]any{
		"id": "did:web:peer.example",
		"verificationMethod": []map[string]any{
			{"id": "did:web:peer.example#ecdh", "publicKeyJwk": testJWK(t, ecdh, nil)},
			{"id": "#referenced", "publicKeyJwk": testJWK(t, referenced, nil)},
		},
		"assertionMethod": []any{
			"#referenced",
			map[string]any{"id": "did:web:peer.example#embedded", "publicKeyJwk": testJWK(t, embedded, nil)},
		},
		"keyAgreement": []any{"#ecdh"},
	})

	for _, methodID := range []string{"#referenced", "did:web:peer.example#referenced"} {
		method, err := doc.MethodFor(PurposeAssertion, methodID)
		if err != nil {
			t.Fatalf("MethodFor(%q): %v", methodID, err)
		}
		key, err := method.ECPublicKey()
		if err != nil || key.X.Cmp(referenced.X) != 0 {
			t.Fatalf("MethodFor(%q) resolved the wrong key: %v", methodID, err)
		}
	}

	// A method published only inside the relationship is still published.
	if _, err := doc.MethodFor(PurposeAssertion, "#embedded"); err != nil {
		t.Fatalf("an embedded relationship entry is a published method: %v", err)
	}

	// Purposes are not interchangeable, in either direction.
	if _, err := doc.MethodFor(PurposeAssertion, "#ecdh"); err == nil {
		t.Fatal("a key published for key agreement may not assert")
	}
	if _, err := doc.MethodFor(PurposeKeyAgreement, "#referenced"); err == nil {
		t.Fatal("a key published for assertions may not receive wrapped keys")
	}
	if !doc.PublishesKeyFor(PurposeAssertion, &referenced.PublicKey) ||
		doc.PublishesKeyFor(PurposeAssertion, &ecdh.PublicKey) {
		t.Fatal("PublishesKeyFor must follow the same relationship split")
	}
}

// The signer's method is found by matching the key, so it need not be first —
// and a key published for key agreement alone cannot become the signing key.
func TestNewDIDDocumentBindsSignerByPublishedRelationship(t *testing.T) {
	signer, other := testKey(t), testKey(t)
	certDER := testCertificate(t, "peer.example", signer)

	doc, err := loadDocument(t, map[string]any{
		"id": "did:web:peer.example",
		"verificationMethod": []map[string]any{
			{"id": "did:web:peer.example#other", "publicKeyJwk": testJWK(t, other, nil)},
			{"id": "did:web:peer.example#identity", "publicKeyJwk": testJWK(t, signer, certDER)},
		},
		"authentication": []any{"#identity"},
		"keyAgreement":   []any{"#other"},
	}, signer)
	if err != nil {
		t.Fatalf("the signer's key is published for authentication: %v", err)
	}
	if got := doc.SigningMethod().ID; got != "did:web:peer.example#identity" {
		t.Fatalf("bound the wrong method: %q", got)
	}
	// The chain validated is the one belonging to that method, not to entry zero.
	if err := doc.VerifyEIDASCertificate(nil); err != nil {
		t.Fatalf("VerifyEIDASCertificate: %v", err)
	}

	_, err = loadDocument(t, map[string]any{
		"id": "did:web:peer.example",
		"verificationMethod": []map[string]any{
			{"id": "did:web:peer.example#identity", "publicKeyJwk": testJWK(t, signer, certDER)},
		},
		"keyAgreement": []any{"#identity"},
	}, signer)
	if err == nil {
		t.Fatal("a held key published for key agreement only must not become the signing key")
	}
}

// The challenge-response names no key, so it is verified against the keys the
// peer publishes for authenticating itself — and against nothing else.
func TestVerifyPeerChallenge(t *testing.T) {
	identityKey, ecdh := testKey(t), testKey(t)
	certDER := testCertificate(t, "peer.example", identityKey)
	docJSON := map[string]any{
		"id": "did:web:peer.example",
		"verificationMethod": []map[string]any{
			{"id": "did:web:peer.example#ecdh", "publicKeyJwk": testJWK(t, ecdh, nil)},
			{"id": "did:web:peer.example#identity", "publicKeyJwk": testJWK(t, identityKey, certDER)},
		},
		"authentication": []any{"#identity"},
		"keyAgreement":   []any{"#ecdh"},
	}

	challenge := []byte("challenge")
	signer, err := loadDocument(t, docJSON, identityKey)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := signer.Sign(challenge)
	if err != nil {
		t.Fatal(err)
	}

	peer := parseDocument(t, docJSON)
	if err := peer.VerifyPeerChallenge(nil, challenge, signature); err != nil {
		t.Fatalf("the peer's authentication key answered the challenge: %v", err)
	}

	// Same key material, published for key agreement instead: nothing in the
	// document may authenticate the peer with it.
	demoted := parseDocument(t, map[string]any{
		"id":                 docJSON["id"],
		"verificationMethod": docJSON["verificationMethod"],
		"keyAgreement":       []any{"#identity", "#ecdh"},
	})
	err = demoted.VerifyPeerChallenge(nil, challenge, signature)
	if err == nil || !strings.Contains(err.Error(), "authenticate its subject") {
		t.Fatalf("a key published for key agreement must not authenticate the peer, got: %v", err)
	}

	// An unpublished key's signature is refused however many keys are on offer.
	stranger, err := loadDocument(t, map[string]any{
		"id": "did:web:peer.example",
		"verificationMethod": []map[string]any{
			{"id": "did:web:peer.example#identity", "publicKeyJwk": testJWK(t, ecdh, nil)},
		},
		"authentication": []any{"#identity"},
	}, ecdh)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := stranger.Sign(challenge)
	if err != nil {
		t.Fatal(err)
	}
	if err := peer.VerifyPeerChallenge(nil, challenge, foreign); err == nil {
		t.Fatal("a response signed by another key must be refused")
	}
}

// A peer that rotates its key-agreement key publishes two, and the wrap names
// the one it was made for instead of the sender demanding there be only one.
func TestPeerKeyAgreementMethodTolerantOfRotation(t *testing.T) {
	current, previous := testKey(t), testKey(t)
	doc := parseDocument(t, map[string]any{
		"id": "did:web:peer.example",
		"verificationMethod": []map[string]any{
			{"id": "did:web:peer.example#ecdh-2027", "publicKeyJwk": testJWK(t, current, nil)},
			{"id": "did:web:peer.example#ecdh-2026", "publicKeyJwk": testJWK(t, previous, nil)},
		},
		"keyAgreement": []any{"#ecdh-2027", "#ecdh-2026"},
	})

	method, err := doc.PeerKeyAgreementMethod()
	if err != nil {
		t.Fatalf("a rotating peer still has a key to wrap to: %v", err)
	}
	if method.ID != "did:web:peer.example#ecdh-2027" {
		t.Fatalf("expected the peer's own first keyAgreement entry, got %q", method.ID)
	}

	none := parseDocument(t, map[string]any{
		"id":                 "did:web:peer.example",
		"verificationMethod": []map[string]any{{"id": "#a", "publicKeyJwk": testJWK(t, current, nil)}},
		"assertionMethod":    []any{"#a"},
	})
	if _, err := none.PeerKeyAgreementMethod(); err == nil {
		t.Fatal("a peer publishing no keyAgreement method cannot receive a wrapped key")
	}
}

func TestOwnKeyAgreementMethodByTokenLabel(t *testing.T) {
	ecdh := testKey(t)
	doc := parseDocument(t, map[string]any{
		"id": "did:web:own.example",
		"verificationMethod": []map[string]any{
			{"id": "did:web:own.example#dcs-ecdh", "publicKeyJwk": testJWK(t, ecdh, nil)},
		},
		"keyAgreement": []any{"#dcs-ecdh"},
	})

	method, err := doc.OwnKeyAgreementMethod("dcs-ecdh")
	if err != nil || method.ID != "did:web:own.example#dcs-ecdh" {
		t.Fatalf("OwnKeyAgreementMethod: %v (%+v)", err, method)
	}
	if _, err := doc.OwnKeyAgreementMethod("dcs-ecdh-unprovisioned"); err == nil {
		t.Fatal("a label the document does not publish must not resolve")
	}
}
