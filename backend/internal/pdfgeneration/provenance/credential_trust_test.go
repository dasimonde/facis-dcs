package provenance

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"digital-contracting-service/internal/base/identity"
)

const testIssuerDID = "did:web:issuer.example"

// issuerDocument builds an issuer's DID document: an identity key under
// authentication (the one the document is bound to) plus the credential key
// published under label, either as an assertionMethod or — to exercise a key that
// is merely present — as a key-agreement key only.
func issuerDocument(t *testing.T, did, label string, credentialKey *ecdsa.PublicKey, publishForAssertion bool) *identity.DIDDocument {
	t.Helper()
	identityKey := newTestKey(t)
	identityMethodID := did + "#dcs-did"
	credentialMethodID := did + "#" + label

	doc := map[string]any{
		"id": did,
		"verificationMethod": []any{
			didMethodJSON(identityMethodID, did, &identityKey.PublicKey),
			didMethodJSON(credentialMethodID, did, credentialKey),
		},
		"authentication": []any{identityMethodID},
	}
	if publishForAssertion {
		doc["assertionMethod"] = []any{credentialMethodID}
	} else {
		doc["keyAgreement"] = []any{credentialMethodID}
	}

	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal did document: %v", err)
	}
	path := filepath.Join(t.TempDir(), "did.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write did document: %v", err)
	}
	parsed, err := identity.NewDIDDocument(path, identityKey)
	if err != nil {
		t.Fatalf("load did document: %v", err)
	}
	return parsed
}

func didMethodJSON(methodID, controller string, key *ecdsa.PublicKey) map[string]any {
	return map[string]any{
		"id":         methodID,
		"type":       "JsonWebKey2020",
		"controller": controller,
		"publicKeyJwk": map[string]any{
			"kty": "EC",
			"crv": "P-256",
			"x":   base64.RawURLEncoding.EncodeToString(key.X.FillBytes(make([]byte, 32))),
			"y":   base64.RawURLEncoding.EncodeToString(key.Y.FillBytes(make([]byte, 32))),
		},
	}
}

// signedCredential issues a credential with a real ecdsa-rdfc-2019 proof naming
// verificationMethod, using the same signer the lifecycle VC path uses.
func signedCredential(t *testing.T, key *ecdsa.PrivateKey, issuer, label string) json.RawMessage {
	t.Helper()
	unsigned := fmt.Sprintf(`{
	  "@context": ["https://www.w3.org/ns/credentials/v2", %q],
	  "type": ["VerifiableCredential"],
	  "id": "urn:dcs:vc:test",
	  "issuer": %q,
	  "validFrom": "2026-01-01T00:00:00Z",
	  "credentialSubject": {"id": "urn:dcs:subject:test"}
	}`, dataIntegrityContext, issuer)

	signed, err := NewHSMVCSigner(key, label).CreateCredential(context.Background(), json.RawMessage(unsigned))
	if err != nil {
		t.Fatalf("sign credential: %v", err)
	}
	return signed
}

func newTestKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return key
}

func TestCredentialVerifierAcceptsAProofByThePublishedAssertionKey(t *testing.T) {
	key := newTestKey(t)
	verifier := &CredentialVerifier{Resolve: func(did string) (*identity.DIDDocument, error) {
		return issuerDocument(t, did, "dcs-vc", &key.PublicKey, true), nil
	}}

	if err := verifier.Verify(signedCredential(t, key, testIssuerDID, "dcs-vc")); err != nil {
		t.Fatalf("a credential signed by the issuer's published assertion key must verify: %v", err)
	}
}

// A credential whose bytes were altered after signing must not verify — the
// check the old presence test could never make.
func TestCredentialVerifierRejectsATamperedCredential(t *testing.T) {
	key := newTestKey(t)
	verifier := &CredentialVerifier{Resolve: func(did string) (*identity.DIDDocument, error) {
		return issuerDocument(t, did, "dcs-vc", &key.PublicKey, true), nil
	}}

	credential := signedCredential(t, key, testIssuerDID, "dcs-vc")
	var parsed map[string]any
	if err := json.Unmarshal(credential, &parsed); err != nil {
		t.Fatalf("unmarshal credential: %v", err)
	}
	parsed["credentialSubject"] = map[string]any{"id": "urn:dcs:subject:somebody-else"}
	tampered, err := json.Marshal(parsed)
	if err != nil {
		t.Fatalf("marshal tampered credential: %v", err)
	}

	err = verifier.Verify(tampered)
	if err == nil {
		t.Fatal("a credential whose subject was rewritten after signing must not verify")
	}
	if CredentialCheck(err) != CheckInvalid {
		t.Fatalf("a failed proof is invalid, not %s", CredentialCheck(err))
	}
}

// A credential that merely parses proves nothing: signed by another key, it is
// exactly the peer-authored credential an inbound verbatim-stored PDF can carry.
func TestCredentialVerifierRejectsAProofByAForeignKey(t *testing.T) {
	published := newTestKey(t)
	foreign := newTestKey(t)
	verifier := &CredentialVerifier{Resolve: func(did string) (*identity.DIDDocument, error) {
		return issuerDocument(t, did, "dcs-vc", &published.PublicKey, true), nil
	}}

	err := verifier.Verify(signedCredential(t, foreign, testIssuerDID, "dcs-vc"))
	if err == nil {
		t.Fatal("a proof made by a key the issuer does not publish must not verify")
	}
	if CredentialCheck(err) != CheckInvalid {
		t.Fatalf("expected invalid, got %s", CredentialCheck(err))
	}
}

// The key must be published FOR assertions: presence in the document is not
// authorization, which is why the resolution goes through assertionMethod.
func TestCredentialVerifierRequiresTheKeyToBePublishedForAssertions(t *testing.T) {
	key := newTestKey(t)
	verifier := &CredentialVerifier{Resolve: func(did string) (*identity.DIDDocument, error) {
		return issuerDocument(t, did, "dcs-ecdh", &key.PublicKey, false), nil
	}}

	err := verifier.Verify(signedCredential(t, key, testIssuerDID, "dcs-ecdh"))
	if err == nil {
		t.Fatal("a key published only for key agreement may not assert a credential")
	}
	if CredentialCheck(err) != CheckInvalid {
		t.Fatalf("expected invalid, got %s", CredentialCheck(err))
	}
}

// An unreachable issuer is not a forged credential and not a valid one: the
// verdict is withheld.
func TestCredentialVerifierReportsAnUnreachableIssuerAsIndeterminate(t *testing.T) {
	key := newTestKey(t)
	verifier := &CredentialVerifier{Resolve: func(string) (*identity.DIDDocument, error) {
		return nil, errors.New("dial tcp: connection refused")
	}}

	err := verifier.Verify(signedCredential(t, key, testIssuerDID, "dcs-vc"))
	if !errors.Is(err, ErrIssuerUnresolved) {
		t.Fatalf("expected ErrIssuerUnresolved, got %v", err)
	}
	if CredentialCheck(err) != CheckIndeterminate {
		t.Fatalf("expected indeterminate, got %s", CredentialCheck(err))
	}
}

// A nil verifier must never read as a pass: no means to verify is the case the
// old presence check silently reported as valid.
func TestNilCredentialVerifierIsIndeterminateNotValid(t *testing.T) {
	var verifier *CredentialVerifier
	err := verifier.Verify(json.RawMessage(`{"proof":{}}`))
	if CredentialCheck(err) != CheckIndeterminate {
		t.Fatalf("expected indeterminate, got %s (err=%v)", CredentialCheck(err), err)
	}
}

func TestCredentialVerifierRejectsANonAssertionProofPurpose(t *testing.T) {
	key := newTestKey(t)
	verifier := &CredentialVerifier{Resolve: func(did string) (*identity.DIDDocument, error) {
		return issuerDocument(t, did, "dcs-vc", &key.PublicKey, true), nil
	}}

	credential := signedCredential(t, key, testIssuerDID, "dcs-vc")
	var parsed map[string]any
	if err := json.Unmarshal(credential, &parsed); err != nil {
		t.Fatalf("unmarshal credential: %v", err)
	}
	proof, _ := parsed["proof"].(map[string]any)
	proof["proofPurpose"] = "authentication"
	repurposed, err := json.Marshal(parsed)
	if err != nil {
		t.Fatalf("marshal credential: %v", err)
	}

	if err := verifier.Verify(repurposed); err == nil {
		t.Fatal("a proof made to authenticate establishes no assertion")
	}
}

// The own document is used without a round trip, and only for the identifier it
// actually belongs to.
func TestCredentialVerifierPrefersItsOwnDocumentForItsOwnCredentials(t *testing.T) {
	key := newTestKey(t)
	own := issuerDocument(t, testIssuerDID, "dcs-vc", &key.PublicKey, true)
	resolved := false
	verifier := &CredentialVerifier{Own: own, Resolve: func(did string) (*identity.DIDDocument, error) {
		resolved = true
		return nil, errors.New("must not be reached")
	}}

	if err := verifier.Verify(signedCredential(t, key, testIssuerDID, "dcs-vc")); err != nil {
		t.Fatalf("own credential must verify against the in-memory document: %v", err)
	}
	if resolved {
		t.Fatal("resolving a credential this instance issued must not require an HTTP round trip")
	}
}
