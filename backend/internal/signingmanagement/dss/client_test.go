package dss

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidatePDFParsesIndication(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/services/rest/validation/validateSignature" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		signed, _ := body["signedDocument"].(map[string]any)
		if signed["bytes"] == "" || signed["name"] != "contract.pdf" {
			t.Fatalf("expected the signed document in the request, got: %v", body)
		}
		// A trimmed WSReportsDTO in the DSS demo webapp's shape.
		_, _ = w.Write([]byte(`{
			"simpleReport": {
				"signatureOrTimestampOrEvidenceRecord": [
					{"Signature": {"Indication": "INDETERMINATE", "SubIndication": "NO_CERTIFICATE_CHAIN_FOUND"}}
				]
			}
		}`))
	}))
	defer srv.Close()

	report, err := New(srv.URL).ValidatePDF(context.Background(), []byte("%PDF-1.7 fake"), "contract.pdf")
	if err != nil {
		t.Fatalf("ValidatePDF: %v", err)
	}
	if report.Indication != "INDETERMINATE" || report.SubIndication != "NO_CERTIFICATE_CHAIN_FOUND" {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestValidatePDFExtractsSignerIdentity(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"simpleReport": {
				"signatureOrTimestampOrEvidenceRecord": [
					{"Signature": {
						"Indication": "TOTAL-PASSED",
						"SignatureFormat": "PAdES-BASELINE-B",
						"SignedBy": "CN=DCS Signatory johndoe,O=Test",
						"SigningTime": "2026-07-18T10:00:00Z"
					}}
				]
			}
		}`))
	}))
	defer srv.Close()

	report, err := New(srv.URL).ValidatePDF(context.Background(), []byte("%PDF fake"), "contract.pdf")
	if err != nil {
		t.Fatalf("ValidatePDF: %v", err)
	}
	if !report.Passed() {
		t.Fatalf("expected TOTAL-PASSED, got %q", report.Indication)
	}
	if report.SignedBy != "CN=DCS Signatory johndoe,O=Test" {
		t.Fatalf("unexpected SignedBy: %q", report.SignedBy)
	}
	if report.SignatureFormat != "PAdES-BASELINE-B" || report.SigningTime == "" {
		t.Fatalf("unexpected format/time: %+v", report)
	}
}

// TestValidatePDFAttributesTheLastSignatureOnAMultiSignedDocument covers a
// multi-signer contract's second-and-later signatory: by the time they
// submit, the document already carries an earlier signatory's signature, so
// simpleReport.signatureOrTimestampOrEvidenceRecord carries one entry per
// signature, oldest first (PAdES incremental updates only append), and
// diagnosticData.Signatures carries a parallel per-signature entry whose
// SigningCertificate.Certificate IDREF resolves the actual signing cert (with
// GivenName/Surname) out of the flat diagnosticData.UsedCertificates list
// (real DSS 6.2 WSReportsDTO shape, confirmed against its published XSDs —
// simpleReport's own Signature/CertificateChain/Certificate types carry no
// GivenName/Surname field at all; SignedBy there is just the cert's CN). The
// report must attribute to the LAST (just-submitted) signature's own
// certificate — the cert↔PID sole-control check (ADR-20) would otherwise
// validate a submission against a different signatory's certificate, or an
// unrelated cert (the CA, the TSA) it happened to visit first.
func TestValidatePDFAttributesTheLastSignatureOnAMultiSignedDocument(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"simpleReport": {
				"signatureOrTimestampOrEvidenceRecord": [
					{"Signature": {
						"Id": "S-A",
						"Indication": "TOTAL-PASSED",
						"SignedBy": "DCS Signatory Instance A Signatory",
						"SigningTime": "2026-07-18T10:00:00Z"
					}},
					{"Timestamp": {
						"Id": "T-1",
						"Indication": "PASSED",
						"SignedBy": "Dev TSA"
					}},
					{"Signature": {
						"Id": "S-B",
						"Indication": "TOTAL-PASSED",
						"SignedBy": "DCS Signatory Instance B Signatory",
						"SigningTime": "2026-07-18T10:05:00Z"
					}}
				]
			},
			"diagnosticData": {
				"Signatures": {
					"Signature": [
						{"Id": "S-A", "SigningCertificate": {"Certificate": "C-A"}},
						{"Id": "S-B", "SigningCertificate": {"Certificate": "C-B"}}
					]
				},
				"UsedCertificates": {
					"Certificate": [
						{"Id": "C-CA", "CommonName": "DCS Wallet Dev Signing CA"},
						{"Id": "C-A", "CommonName": "DCS Signatory Instance A Signatory", "GivenName": "Instance A Signatory", "Surname": "BDD-Testperson"},
						{"Id": "C-B", "CommonName": "DCS Signatory Instance B Signatory", "GivenName": "Instance B Signatory", "Surname": "BDD-Testperson"}
					]
				}
			}
		}`))
	}))
	defer srv.Close()

	report, err := New(srv.URL).ValidatePDF(context.Background(), []byte("%PDF fake"), "contract.pdf")
	if err != nil {
		t.Fatalf("ValidatePDF: %v", err)
	}
	if report.SubjectGivenName() != "Instance B Signatory" || report.SubjectSurname() != "BDD-Testperson" {
		t.Fatalf("expected the LAST signature's own certificate identity (Instance B Signatory/BDD-Testperson), got %+v", report)
	}
}

// TestValidatePDFResolvesIdentityForASingleSignature is the ordinary,
// single-signer case (no incremental prior signature to disambiguate from) —
// it must still resolve GivenName/Surname via diagnosticData, since
// simpleReport never carries them regardless of how many signatures a
// document has.
func TestValidatePDFResolvesIdentityForASingleSignature(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"simpleReport": {
				"signatureOrTimestampOrEvidenceRecord": [
					{"Signature": {
						"Id": "S-1",
						"Indication": "TOTAL-PASSED",
						"SignedBy": "DCS Signatory SignerThirteen"
					}}
				]
			},
			"diagnosticData": {
				"Signatures": {
					"Signature": {"Id": "S-1", "SigningCertificate": {"Certificate": "C-1"}}
				},
				"UsedCertificates": {
					"Certificate": [
						{"Id": "C-CA", "CommonName": "DCS Wallet Dev Signing CA"},
						{"Id": "C-1", "CommonName": "DCS Signatory SignerThirteen", "GivenName": "SignerThirteen", "Surname": "BDD-Testperson"}
					]
				}
			}
		}`))
	}))
	defer srv.Close()

	report, err := New(srv.URL).ValidatePDF(context.Background(), []byte("%PDF fake"), "contract.pdf")
	if err != nil {
		t.Fatalf("ValidatePDF: %v", err)
	}
	if report.SubjectGivenName() != "SignerThirteen" || report.SubjectSurname() != "BDD-Testperson" {
		t.Fatalf("expected SignerThirteen/BDD-Testperson, got %+v", report)
	}
}

func TestAssertValidAES(t *testing.T) {
	// A cryptographically sound signature with a signing certificate is a valid
	// AES. Identifying the signatory is the ceremony PID's job, not a certificate
	// subject match (eIDAS Art. 26 mandates no PID-to-cert binding).
	passed := &Report{Indication: "TOTAL-PASSED", SignedBy: "CN=Jane Doe, SURNAME=Doe, GIVENNAME=Jane"}
	if err := passed.AssertValidAES(); err != nil {
		t.Fatalf("expected a valid AES to be accepted: %v", err)
	}

	// AES: a non-qualified CA yields INDETERMINATE/NO_CERTIFICATE_CHAIN_FOUND
	// (a trust gap, not a crypto failure) and MUST still be accepted — qualified
	// trust is a QES property, not required for AES.
	nonQualified := &Report{Indication: "INDETERMINATE", SubIndication: "NO_CERTIFICATE_CHAIN_FOUND", SignedBy: "CN=Jane Doe"}
	if err := nonQualified.AssertValidAES(); err != nil {
		t.Fatalf("expected a cryptographically-sound AES over a non-qualified CA to be accepted: %v", err)
	}

	failed := &Report{Indication: "TOTAL-FAILED", SubIndication: "HASH_FAILURE", SignedBy: "CN=x"}
	if err := failed.AssertValidAES(); err == nil {
		t.Fatal("expected a failed indication to be rejected")
	}

	// A crypto failure is rejected even when the top indication is INDETERMINATE.
	cryptoBroken := &Report{Indication: "INDETERMINATE", SubIndication: "SIG_CRYPTO_FAILURE", SignedBy: "CN=x"}
	if err := cryptoBroken.AssertValidAES(); err == nil {
		t.Fatal("expected a crypto failure to be rejected")
	}

	noCert := &Report{Indication: "TOTAL-PASSED"}
	if err := noCert.AssertValidAES(); err == nil {
		t.Fatal("expected rejection when no signing certificate is present")
	}
}

func TestAssertValidQES(t *testing.T) {
	qualified := &Report{Indication: "TOTAL-PASSED", SignedBy: "CN=Jane Doe, SURNAME=Doe, GIVENNAME=Jane", Qualification: "QESIG"}
	if err := qualified.AssertValidQES(); err != nil {
		t.Fatalf("expected a TOTAL-PASSED QESIG signature to be accepted as QES: %v", err)
	}

	// AES's relaxation does NOT carry over to QES: a trust-chain gap that AES
	// tolerates must disqualify a QES claim.
	nonQualifiedChain := &Report{Indication: "INDETERMINATE", SubIndication: "NO_CERTIFICATE_CHAIN_FOUND", SignedBy: "CN=Jane Doe"}
	if err := nonQualifiedChain.AssertValidQES(); err == nil {
		t.Fatal("expected an INDETERMINATE/NO_CERTIFICATE_CHAIN_FOUND signature to be rejected for QES")
	}

	// TOTAL-PASSED alone is not enough — an advanced (non-qualified) signature
	// must not pass the QES gate just because the chain validated.
	advancedOnly := &Report{Indication: "TOTAL-PASSED", SignedBy: "CN=Jane Doe", Qualification: "ADESIG"}
	if err := advancedOnly.AssertValidQES(); err == nil {
		t.Fatal("expected a non-qualified TOTAL-PASSED signature to be rejected for QES")
	}

	failed := &Report{Indication: "TOTAL-FAILED", SubIndication: "HASH_FAILURE", SignedBy: "CN=x", Qualification: "QESIG"}
	if err := failed.AssertValidQES(); err == nil {
		t.Fatal("expected a failed indication to be rejected for QES")
	}
}

func TestAssertMeetsLevel(t *testing.T) {
	qualified := &Report{Indication: "TOTAL-PASSED", SignedBy: "CN=Jane Doe", Qualification: "QESIG"}
	if err := qualified.AssertMeetsLevel("QES"); err != nil {
		t.Fatalf("expected QESIG to satisfy a QES requirement: %v", err)
	}

	nonQualified := &Report{Indication: "INDETERMINATE", SubIndication: "NO_CERTIFICATE_CHAIN_FOUND", SignedBy: "CN=Jane Doe"}
	if err := nonQualified.AssertMeetsLevel("QES"); err == nil {
		t.Fatal("expected a non-qualified signature to fail a QES requirement")
	}
	if err := nonQualified.AssertMeetsLevel("AES"); err != nil {
		t.Fatalf("expected the same signature to satisfy an AES requirement: %v", err)
	}
	if err := nonQualified.AssertMeetsLevel(""); err != nil {
		t.Fatalf("expected an unspecified requirement to fall back to the AES gate: %v", err)
	}
}

func TestParseSubjectAttributesAndAccessors(t *testing.T) {
	r := &Report{SignedBy: "CN=Jane Doe, SURNAME=Doe, GIVENNAME=Jane"}
	if got := r.SubjectGivenName(); got != "Jane" {
		t.Fatalf("SubjectGivenName: got %q", got)
	}
	if got := r.SubjectSurname(); got != "Doe" {
		t.Fatalf("SubjectSurname: got %q", got)
	}

	// Structured fields (as real DSS diagnostic data reports them) take
	// precedence over parsing the SignedBy DN string.
	structured := &Report{SignedBy: "CN=Jane Doe", GivenName: "Structured", Surname: "Fields"}
	if got := structured.SubjectGivenName(); got != "Structured" {
		t.Fatalf("expected structured GivenName to win, got %q", got)
	}
	if got := structured.SubjectSurname(); got != "Fields" {
		t.Fatalf("expected structured Surname to win, got %q", got)
	}
}

func TestValidatePDFHardFailsWhenUnreachable(t *testing.T) {
	// A configured DSS that cannot be reached is an error, never a silent skip.
	if _, err := New("http://127.0.0.1:1").ValidatePDF(context.Background(), []byte("x"), "x.pdf"); err == nil {
		t.Fatal("expected an error for an unreachable DSS")
	}
}

func TestValidatePDFRejectsReportWithoutIndication(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"simpleReport": {}}`))
	}))
	defer srv.Close()
	if _, err := New(srv.URL).ValidatePDF(context.Background(), []byte("x"), "x.pdf"); err == nil {
		t.Fatal("expected an error for a response without an Indication")
	}
}
