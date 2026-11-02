package query

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"digital-contracting-service/internal/pdfgeneration/provenance"
	"digital-contracting-service/internal/signingmanagement/dss"
)

// fakeDSS answers ValidatePDFSignatures with fixed reports.
type fakeDSS struct {
	reports []*dss.Report
	err     error
	called  bool
}

func (f *fakeDSS) ValidatePDFSignatures(context.Context, []byte) ([]*dss.Report, error) {
	f.called = true
	return f.reports, f.err
}

const signedTestPDF = "%PDF- 9 0 obj << /Type /Sig /ByteRange [0 1 2 3] >> endobj"

// The verifier checks the PDF signatures (DCS-OR-C2PA-006) through the EU DSS
// that /signature/validate uses, and says so: every signature must be a sound
// advanced electronic signature, and the indication of each is reported.
func TestRunVerifyReportsPAdESIndicationFromDSS(t *testing.T) {
	pdfCore := newStubPDFCore(t, http.StatusOK, map[string]any{"match": true, "c2pa_signature_valid": true})
	passed := &dss.Report{Indication: "TOTAL-PASSED", SignedBy: "Signatory A"}
	trustGap := &dss.Report{Indication: "INDETERMINATE", SubIndication: "NO_CERTIFICATE_CHAIN_FOUND", SignedBy: "Signatory B"}
	broken := &dss.Report{Indication: "TOTAL-FAILED", SubIndication: "HASH_FAILURE", SignedBy: "Signatory B"}

	for _, tc := range []struct {
		name      string
		pdf       string
		validator PDFSignatureValidator
		status    string
		detail    string
	}{
		{"every signature sound", signedTestPDF, &fakeDSS{reports: []*dss.Report{passed, trustGap}}, provenance.CheckValid, "signature 2 by \"Signatory B\": INDETERMINATE/NO_CERTIFICATE_CHAIN_FOUND"},
		{"one signature broken", signedTestPDF, &fakeDSS{reports: []*dss.Report{passed, broken}}, provenance.CheckInvalid, "TOTAL-FAILED/HASH_FAILURE"},
		{"DSS unreachable", signedTestPDF, &fakeDSS{err: errors.New("dss: validation request failed")}, provenance.CheckIndeterminate, "validation request failed"},
		{"no DSS configured", signedTestPDF, nil, provenance.CheckIndeterminate, "DSS_URL"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := runVerify(context.Background(), []byte(tc.pdf), pdfCore, nil, nil, tc.validator, SignerAnchors{}, "active")
			if err != nil {
				t.Fatalf("runVerify: %v", err)
			}
			if result.PdfSignatureStatus != tc.status {
				t.Fatalf("pdf_signature_status = %q, want %q", result.PdfSignatureStatus, tc.status)
			}
			if result.PdfSignatureSource == nil || *result.PdfSignatureSource != "eu-dss" {
				t.Fatalf("pdf_signature_source = %v, want eu-dss", result.PdfSignatureSource)
			}
			if result.PdfSignatureDetail == nil || !strings.Contains(*result.PdfSignatureDetail, tc.detail) {
				t.Fatalf("pdf_signature_detail = %v, want it to contain %q", result.PdfSignatureDetail, tc.detail)
			}
		})
	}
}

// A PDF without a PAdES signature has no PDF signature to check, and the DSS
// is not asked.
func TestRunVerifyReportsAnUnsignedPDFSignatureAsNotAvailable(t *testing.T) {
	pdfCore := newStubPDFCore(t, http.StatusOK, map[string]any{"match": true, "c2pa_signature_valid": true})
	validator := &fakeDSS{}

	result, err := runVerify(context.Background(), []byte("%PDF-"), pdfCore, nil, nil, validator, SignerAnchors{}, "draft")
	if err != nil {
		t.Fatalf("runVerify: %v", err)
	}
	if result.PdfSignatureStatus != provenance.CheckNotAvailable || result.PdfSignatureSource != nil {
		t.Fatalf("pdf signature = %q from %v, want not_available from nothing", result.PdfSignatureStatus, result.PdfSignatureSource)
	}
	if validator.called {
		t.Fatal("the DSS was asked about a PDF without a signature")
	}
}
