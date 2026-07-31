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
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/pdfgeneration/provenance"
	"digital-contracting-service/internal/signingmanagement/db"
)

const evidenceIssuerDID = "did:web:issuer.example"

// pdfOnlyRepo answers only FetchContractPDFBytes; any other repo call this test
// path makes is a bug the panic surfaces.
type pdfOnlyRepo struct {
	db.ContractRepo
	pdf []byte
}

func (r pdfOnlyRepo) FetchContractPDFBytes(context.Context, *sqlx.Tx, string) ([]byte, error) {
	return r.pdf, nil
}

func evidenceTestKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return key
}

func evidenceIssuerDocument(t *testing.T, credentialKey *ecdsa.PublicKey) *identity.DIDDocument {
	t.Helper()
	identityKey := evidenceTestKey(t)
	method := func(id string, key *ecdsa.PublicKey) map[string]any {
		return map[string]any{
			"id":         id,
			"type":       "JsonWebKey2020",
			"controller": evidenceIssuerDID,
			"publicKeyJwk": map[string]any{
				"kty": "EC",
				"crv": "P-256",
				"x":   base64.RawURLEncoding.EncodeToString(key.X.FillBytes(make([]byte, 32))),
				"y":   base64.RawURLEncoding.EncodeToString(key.Y.FillBytes(make([]byte, 32))),
			},
		}
	}
	raw, err := json.Marshal(map[string]any{
		"id": evidenceIssuerDID,
		"verificationMethod": []any{
			method(evidenceIssuerDID+"#dcs-did", &identityKey.PublicKey),
			method(evidenceIssuerDID+"#dcs-vc", credentialKey),
		},
		"authentication":  []any{evidenceIssuerDID + "#dcs-did"},
		"assertionMethod": []any{evidenceIssuerDID + "#dcs-vc"},
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

// signingSummary issues a real ContractSigningSummaryCredential, the document the
// compliance viewer, the signer cross-check and the SHACL-drift verdict all read.
func signingSummary(t *testing.T, key *ecdsa.PrivateKey, signerDID, reportHash string) json.RawMessage {
	t.Helper()
	vc, _, err := provenance.IssueSigningSummaryVC(context.Background(),
		provenance.NewHSMVCSigner(key, "dcs-vc"), evidenceIssuerDID, provenance.SigningSummary{
			ContractID:           "did:example:contract-1",
			SignerDID:            signerDID,
			CeremonyID:           "ceremony-1",
			FieldName:            "party-1",
			ContentHash:          "aa",
			PDFHash:              "bb",
			CredentialType:       "AES",
			KBSDHash:             "cc",
			ValidationReportHash: reportHash,
		})
	if err != nil {
		t.Fatalf("issue signing summary: %v", err)
	}
	return vc
}

// evidenceValidator wires a Validator whose pdf-core stub returns the given
// evidence attachment for the stored PDF.
func evidenceValidator(t *testing.T, attachment []byte, verifier *provenance.CredentialVerifier) *Validator {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/evidence/extract" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if len(attachment) == 0 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(attachment)
	}))
	t.Cleanup(srv.Close)

	return &Validator{
		CRepo:       pdfOnlyRepo{pdf: []byte("%PDF-stored")},
		PDFCore:     pdfcore.New(srv.URL, func([]byte) ([]byte, error) { return make([]byte, 64), nil }),
		Credentials: verifier,
	}
}

func TestSigningEvidenceIsUsedOnlyAfterItsProofVerifies(t *testing.T) {
	key := evidenceTestKey(t)
	summary := signingSummary(t, key, "did:example:signer-1", "report-hash")
	validator := evidenceValidator(t, summary, &provenance.CredentialVerifier{
		Resolve: func(string) (*identity.DIDDocument, error) { return evidenceIssuerDocument(t, &key.PublicKey), nil },
	})

	evidence := validator.readSigningEvidence(context.Background(), nil, "did:example:contract-1")
	if len(evidence.Documents) != 1 {
		t.Fatalf("a summary signed by the issuer's assertion key must be usable, got %d documents (findings %v)",
			len(evidence.Documents), evidence.Findings)
	}
	if len(evidence.Findings) != 0 {
		t.Fatalf("a verified summary must raise no finding, got %v", evidence.Findings)
	}
	if collected := collectSigningEvidence(evidence.Documents); len(collected) != 1 || collected[0].SignerDID == "" {
		t.Fatalf("the compliance viewer must see the verified signer, got %+v", collected)
	}
}

// The audit finding: the local readers accepted whatever parsed. A summary signed
// by a key the issuer does not publish is exactly what an inbound verbatim-stored
// PDF can carry, and none of its claims may be used.
func TestSigningEvidenceSignedByAForeignKeyIsRefusedAndReported(t *testing.T) {
	published := evidenceTestKey(t)
	foreign := evidenceTestKey(t)
	summary := signingSummary(t, foreign, "did:example:attacker", "forged-hash")
	validator := evidenceValidator(t, summary, &provenance.CredentialVerifier{
		Resolve: func(string) (*identity.DIDDocument, error) {
			return evidenceIssuerDocument(t, &published.PublicKey), nil
		},
	})

	evidence := validator.readSigningEvidence(context.Background(), nil, "did:example:contract-1")
	if len(evidence.Documents) != 0 {
		t.Fatal("a summary whose proof does not verify must not be used")
	}
	if len(evidence.Findings) != 1 || !strings.Contains(evidence.Findings[0], "invalid") {
		t.Fatalf("the refusal must be reported as a finding, got %v", evidence.Findings)
	}
	// The three consumers of the evidence must all fall back to "nothing to
	// report" rather than reading the unverified claims.
	if collected := collectSigningEvidence(evidence.Documents); len(collected) != 0 {
		t.Fatalf("the compliance viewer must not surface unverified evidence, got %+v", collected)
	}
	if findings := validator.crossCheckEmbeddedPID(context.Background(), nil, "did:example:contract-1", evidence.Documents); findings != nil {
		t.Fatalf("the signer cross-check must not confirm anything from unverified evidence, got %v", findings)
	}
	if findings := validator.crossCheckSHACLDrift(context.Background(), nil, "did:example:contract-1", evidence.Documents); findings != nil {
		t.Fatalf("the SHACL-drift verdict must not compare against an unverified hash, got %v", findings)
	}
}

// An unreachable issuer is neither a forged summary nor a usable one.
func TestSigningEvidenceWithAnUnresolvableIssuerIsIndeterminate(t *testing.T) {
	key := evidenceTestKey(t)
	summary := signingSummary(t, key, "did:example:signer-1", "report-hash")
	validator := evidenceValidator(t, summary, &provenance.CredentialVerifier{
		Resolve: func(string) (*identity.DIDDocument, error) { return nil, errors.New("dial tcp: connection refused") },
	})

	evidence := validator.readSigningEvidence(context.Background(), nil, "did:example:contract-1")
	if len(evidence.Documents) != 0 {
		t.Fatal("evidence this instance cannot verify must not be used")
	}
	if len(evidence.Findings) != 1 || !strings.Contains(evidence.Findings[0], "indeterminate") {
		t.Fatalf("expected an indeterminate finding, got %v", evidence.Findings)
	}
}

// A bundle: only the members that verify survive, and the rest are named.
func TestSigningEvidenceBundleKeepsOnlyVerifiedMembers(t *testing.T) {
	key := evidenceTestKey(t)
	foreign := evidenceTestKey(t)
	bundle, err := json.Marshal([]json.RawMessage{
		signingSummary(t, key, "did:example:signer-1", "report-hash"),
		signingSummary(t, foreign, "did:example:attacker", "forged-hash"),
	})
	if err != nil {
		t.Fatalf("marshal bundle: %v", err)
	}
	validator := evidenceValidator(t, bundle, &provenance.CredentialVerifier{
		Resolve: func(string) (*identity.DIDDocument, error) { return evidenceIssuerDocument(t, &key.PublicKey), nil },
	})

	evidence := validator.readSigningEvidence(context.Background(), nil, "did:example:contract-1")
	if len(evidence.Documents) != 1 {
		t.Fatalf("expected exactly the verified member to survive, got %d", len(evidence.Documents))
	}
	if len(evidence.Findings) != 1 {
		t.Fatalf("expected the unverified member to be reported, got %v", evidence.Findings)
	}
	if hash := signingSummarySHACLHash(evidence.Documents[0]); hash != "report-hash" {
		t.Fatalf("the drift comparison must use the verified member's hash, got %q", hash)
	}
}

// A corrupted attachment (the tamper scenario's seam) is reported rather than
// silently yielding no evidence.
func TestCorruptedSigningEvidenceIsReported(t *testing.T) {
	validator := evidenceValidator(t, []byte{0x01, 0x02, 0x03}, &provenance.CredentialVerifier{
		Resolve: func(string) (*identity.DIDDocument, error) { return nil, errors.New("not reached") },
	})

	evidence := validator.readSigningEvidence(context.Background(), nil, "did:example:contract-1")
	if len(evidence.Documents) != 0 {
		t.Fatal("undecodable evidence must not be used")
	}
	if len(evidence.Findings) == 0 {
		t.Fatal("undecodable evidence must be reported")
	}
	if joined := strings.ToLower(fmt.Sprint(evidence.Findings)); !strings.Contains(joined, "evidence") {
		t.Fatalf("the finding must name the evidence, got %v", evidence.Findings)
	}
}

func TestNoSigningEvidenceYieldsNoFindings(t *testing.T) {
	validator := evidenceValidator(t, nil, &provenance.CredentialVerifier{
		Resolve: func(string) (*identity.DIDDocument, error) { return nil, errors.New("not reached") },
	})

	evidence := validator.readSigningEvidence(context.Background(), nil, "did:example:contract-1")
	if len(evidence.Documents) != 0 || len(evidence.Findings) != 0 {
		t.Fatalf("an unsigned contract reports nothing, got %+v", evidence)
	}
}
