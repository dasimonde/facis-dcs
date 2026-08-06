package query

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/pdfgeneration/provenance"
	"digital-contracting-service/internal/pdfgeneration/provenance/provenancetest"
)

const testIssuerDID = "did:web:issuer.example"

func testKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return key
}

// testIssuerDocument publishes credentialKey as the issuer's assertion method,
// alongside a separate identity key the document is bound to.
func testIssuerDocument(t *testing.T, credentialKey *ecdsa.PublicKey) *identity.DIDDocument {
	t.Helper()
	identityKey := testKey(t)
	method := func(id string, key *ecdsa.PublicKey) map[string]any {
		return map[string]any{
			"id":         id,
			"type":       "JsonWebKey2020",
			"controller": testIssuerDID,
			"publicKeyJwk": map[string]any{
				"kty": "EC",
				"crv": "P-256",
				"x":   base64.RawURLEncoding.EncodeToString(key.X.FillBytes(make([]byte, 32))),
				"y":   base64.RawURLEncoding.EncodeToString(key.Y.FillBytes(make([]byte, 32))),
			},
		}
	}
	raw, err := json.Marshal(map[string]any{
		"id": testIssuerDID,
		"verificationMethod": []any{
			method(testIssuerDID+"#dcs-did", &identityKey.PublicKey),
			method(testIssuerDID+"#dcs-vc", credentialKey),
		},
		"authentication":  []any{testIssuerDID + "#dcs-did"},
		"assertionMethod": []any{testIssuerDID + "#dcs-vc"},
	})
	if err != nil {
		t.Fatalf("marshal did document: %v", err)
	}
	path := filepath.Join(t.TempDir(), "did.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write did document: %v", err)
	}
	doc, err := identity.NewDIDDocument(path, identityKey)
	if err != nil {
		t.Fatalf("load did document: %v", err)
	}
	return doc
}

// testLifecycleCredential issues a lifecycle credential whose credentialStatus
// points at statusListURL, signed by key under the #dcs-vc method.
func testLifecycleCredential(t *testing.T, key *ecdsa.PrivateKey, statusListURL string) []byte {
	t.Helper()
	unsigned := fmt.Sprintf(`{
	  "@context": ["https://www.w3.org/ns/credentials/v2", "https://w3id.org/security/data-integrity/v2"],
	  "type": ["VerifiableCredential", "ContractLifecycleCredential"],
	  "id": "urn:dcs:vc:test",
	  "issuer": %q,
	  "validFrom": "2026-01-01T00:00:00Z",
	  "credentialSubject": {"id": "urn:dcs:subject:test", "status": "active"},
	  "credentialStatus": {
	    "id": %q,
	    "type": "TokenStatusList",
	    "statusPurpose": "revocation",
	    "statusListIndex": "7",
	    "statusListCredential": %q
	  }
	}`, testIssuerDID, statusListURL+"#7", statusListURL)

	signed, err := provenance.NewHSMVCSigner(key, "dcs-vc").CreateCredential(context.Background(), json.RawMessage(unsigned))
	if err != nil {
		t.Fatalf("sign lifecycle credential: %v", err)
	}
	return signed
}

// newStatusListServer serves a status list the way a deployment serves its own
// — signed, with a chain whose leaf names the issuer — and records whether the
// revocation lookup was made at all, the observable difference between
// following a credential's own status pointer and refusing to.
func newStatusListServer(t *testing.T) *provenancetest.SignedStatusList {
	t.Helper()
	return provenancetest.NewSignedStatusList(t)
}

// newStubPDFCore answers POST /verify with the given status and body.
func newStubPDFCore(t *testing.T, verifyStatus int, verifyBody map[string]any) *pdfcore.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/verify" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(verifyStatus)
		_ = json.NewEncoder(w).Encode(verifyBody)
	}))
	t.Cleanup(srv.Close)
	return pdfcore.New(srv.URL, func([]byte) ([]byte, error) { return make([]byte, 64), nil })
}

// A verified credential is the only one whose own credentialStatus is followed,
// and the only one reported as a passing check.
func TestRunVerifyReportsAVerifiedLifecycleCredentialAsValid(t *testing.T) {
	key := testKey(t)
	statusList := newStatusListServer(t)
	pdfCore := newStubPDFCore(t, http.StatusOK, map[string]any{
		"match":                true,
		"c2pa_signature_valid": true,
		"vc_present":           true,
		"vc_bytes":             base64.StdEncoding.EncodeToString(testLifecycleCredential(t, key, statusList.ListURI)),
	})
	verifier := &provenance.CredentialVerifier{Resolve: func(string) (*identity.DIDDocument, error) {
		return testIssuerDocument(t, &key.PublicKey), nil
	}}

	result, err := runVerify(context.Background(), []byte("%PDF-"), pdfCore, verifier, statusList.Verifier, "active")
	if err != nil {
		t.Fatalf("runVerify: %v", err)
	}
	if result.VcProofStatus != provenance.CheckValid {
		t.Fatalf("vc_proof_status = %q, want valid", result.VcProofStatus)
	}
	if result.C2paSignatureStatus != provenance.CheckValid {
		t.Fatalf("c2pa_signature_status = %q, want valid", result.C2paSignatureStatus)
	}
	if !statusList.Fetched() {
		t.Fatal("a verified credential's own status pointer must be followed")
	}
}

// The case the old presence check reported as a pass: a credential that parses,
// whose issuer cannot be resolved. The verdict is withheld, and the revocation
// lookup — which follows a pointer inside that credential — does not run.
func TestRunVerifyReportsAnUnresolvableIssuerAsIndeterminate(t *testing.T) {
	key := testKey(t)
	statusList := newStatusListServer(t)
	pdfCore := newStubPDFCore(t, http.StatusOK, map[string]any{
		"match":                true,
		"c2pa_signature_valid": true,
		"vc_present":           true,
		"vc_bytes":             base64.StdEncoding.EncodeToString(testLifecycleCredential(t, key, statusList.ListURI)),
	})
	verifier := &provenance.CredentialVerifier{Resolve: func(string) (*identity.DIDDocument, error) {
		return nil, errors.New("dial tcp: connection refused")
	}}

	result, err := runVerify(context.Background(), []byte("%PDF-"), pdfCore, verifier, statusList.Verifier, "active")
	if err != nil {
		t.Fatalf("runVerify: %v", err)
	}
	if result.VcProofStatus != provenance.CheckIndeterminate {
		t.Fatalf("vc_proof_status = %q, want indeterminate", result.VcProofStatus)
	}
	if result.StatusListStatus == nil || *result.StatusListStatus == "" {
		t.Fatal("an unverified credential leaves the revocation state unknown, which must be said")
	}
	if statusList.Fetched() {
		t.Fatal("the revocation lookup must not follow a pointer inside an unverified credential")
	}
}

// A credential signed by a key its issuer does not publish is invalid, not merely
// unverified, and its status pointer is not followed either.
func TestRunVerifyReportsAForeignSignedCredentialAsInvalid(t *testing.T) {
	published := testKey(t)
	foreign := testKey(t)
	statusList := newStatusListServer(t)
	pdfCore := newStubPDFCore(t, http.StatusOK, map[string]any{
		"match":                true,
		"c2pa_signature_valid": true,
		"vc_present":           true,
		"vc_bytes":             base64.StdEncoding.EncodeToString(testLifecycleCredential(t, foreign, statusList.ListURI)),
	})
	verifier := &provenance.CredentialVerifier{Resolve: func(string) (*identity.DIDDocument, error) {
		return testIssuerDocument(t, &published.PublicKey), nil
	}}

	result, err := runVerify(context.Background(), []byte("%PDF-"), pdfCore, verifier, statusList.Verifier, "active")
	if err != nil {
		t.Fatalf("runVerify: %v", err)
	}
	if result.VcProofStatus != provenance.CheckInvalid {
		t.Fatalf("vc_proof_status = %q, want invalid", result.VcProofStatus)
	}
	if statusList.Fetched() {
		t.Fatal("the revocation lookup must not follow a pointer inside an unverified credential")
	}
}

func TestRunVerifyReportsAnAbsentCredentialAsNotAvailable(t *testing.T) {
	pdfCore := newStubPDFCore(t, http.StatusOK, map[string]any{
		"match":                true,
		"c2pa_signature_valid": true,
		"vc_present":           false,
	})

	result, err := runVerify(context.Background(), []byte("%PDF-"), pdfCore, nil, nil, "draft")
	if err != nil {
		t.Fatalf("runVerify: %v", err)
	}
	if result.VcProofStatus != provenance.CheckNotAvailable {
		t.Fatalf("vc_proof_status = %q, want not_available", result.VcProofStatus)
	}
}

// pdf-core now answers with a real claim-signature verdict, so a failed one must
// surface as invalid rather than as the literal true this field used to carry.
func TestRunVerifyReportsAFailedC2PAClaimSignatureAsInvalid(t *testing.T) {
	pdfCore := newStubPDFCore(t, http.StatusOK, map[string]any{
		"match":                true,
		"c2pa_signature_valid": false,
		"c2pa_signature_error": "manifest urn:c2pa:x: COSE_Sign1 claim signature does not verify against the x5chain leaf key",
		"vc_present":           false,
	})

	result, err := runVerify(context.Background(), []byte("%PDF-"), pdfCore, nil, nil, "active")
	if err != nil {
		t.Fatalf("runVerify: %v", err)
	}
	if result.C2paSignatureStatus != provenance.CheckInvalid {
		t.Fatalf("c2pa_signature_status = %q, want invalid", result.C2paSignatureStatus)
	}
}

// A /verify that never returned a body carries no claim-signature verdict at all,
// which is not the same as a failed one.
func TestRunVerifyReportsNoC2PAVerdictWhenVerifyDidNotAnswer(t *testing.T) {
	pdfCore := newStubPDFCore(t, http.StatusConflict, map[string]any{})

	result, err := runVerify(context.Background(), []byte("%PDF-"), pdfCore, nil, nil, "active")
	if err != nil {
		t.Fatalf("runVerify: %v", err)
	}
	if result.C2paSignatureStatus != provenance.CheckNotAvailable {
		t.Fatalf("c2pa_signature_status = %q, want not_available", result.C2paSignatureStatus)
	}
	if result.Match {
		t.Fatal("a 409 from /verify is a content mismatch, not a match")
	}
}

// The three hash fields are Required on PDFVerifyResult and nothing ever set
// them, so every response carried "" — which reads to a caller as "the hash is
// blank" rather than "this is not computed". pdf-core reports them; they have to
// reach the wire.
func TestRunVerifyCarriesThePDFCoreDigests(t *testing.T) {
	pdfCore := newStubPDFCore(t, http.StatusOK, map[string]any{
		"match":                true,
		"c2pa_signature_valid": true,
		"vc_present":           false,
		"jsonld_hash":          "1111",
		"base_pdf_hash":        "2222",
		"stored_base_pdf_hash": "2222",
	})

	result, err := runVerify(context.Background(), []byte("%PDF-"), pdfCore, nil, nil, "active")
	if err != nil {
		t.Fatalf("runVerify: %v", err)
	}
	if result.JsonldHash != "1111" {
		t.Errorf("jsonld_hash = %q, want 1111", result.JsonldHash)
	}
	if result.BasePdfHash != "2222" {
		t.Errorf("base_pdf_hash = %q, want 2222", result.BasePdfHash)
	}
	if result.StoredBasePdfHash != "2222" {
		t.Errorf("stored_base_pdf_hash = %q, want 2222", result.StoredBasePdfHash)
	}
}

// A tampered artifact is the case the digests exist for: the verdict is a
// mismatch and the two PDF hashes say which side diverged. Reporting match=false
// with three empty hashes would leave the finding unevidenced.
func TestRunVerifyReportsDivergingDigestsOnAContentMismatch(t *testing.T) {
	pdfCore := newStubPDFCore(t, http.StatusConflict, map[string]any{
		"name":                 "conflict",
		"message":              "embedded payload does not reproduce the submitted PDF",
		"jsonld_hash":          "1111",
		"base_pdf_hash":        "2222",
		"stored_base_pdf_hash": "3333",
	})

	result, err := runVerify(context.Background(), []byte("%PDF-"), pdfCore, nil, nil, "active")
	if err != nil {
		t.Fatalf("runVerify: %v", err)
	}
	if result.Match {
		t.Fatal("a 409 from /verify is a content mismatch, not a match")
	}
	if result.BasePdfHash == result.StoredBasePdfHash {
		t.Errorf("a content mismatch must report diverging PDF digests: base=%q stored=%q",
			result.BasePdfHash, result.StoredBasePdfHash)
	}
	if result.JsonldHash != "1111" {
		t.Errorf("jsonld_hash = %q, want 1111 — the payload is still identified", result.JsonldHash)
	}
}
