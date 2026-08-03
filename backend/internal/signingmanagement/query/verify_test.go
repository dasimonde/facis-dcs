package query

import (
	"errors"
	"strings"
	"testing"

	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/pdfgeneration/provenance"
)

// A refused /verify returns no body, so no credential bytes come back and the
// credential check never runs. Reporting that as "missing from the PDF" states
// something about the document that nothing established — it sent a federated
// contract's reader hunting for an embedding regression that was not there,
// while the actual refusal reason was dropped on the floor.
func TestVerifyFindingsDoNotInventADefectFromARefusedCheck(t *testing.T) {
	findings := verifyFindings(verifyFindingInputs{
		VerifyErr:           errors.New("pdf-core /verify: status 409: content mismatch"),
		C2PAManifestFound:   true,
		C2PASignatureStatus: provenance.CheckNotAvailable,
		VCProofStatus:       provenance.CheckNotAvailable,
	})

	joined := strings.Join(findings, " | ")
	if strings.Contains(joined, "Contract lifecycle credential missing from the PDF") {
		t.Errorf("a refused check must not be reported as a missing credential: %s", joined)
	}
	if !strings.Contains(joined, "status 409: content mismatch") {
		t.Errorf("the reason pdf-core refused must be reported: %s", joined)
	}
}

// The finding stays for the case it was written for: pdf-core answered, and the
// document it read carried no lifecycle credential.
func TestVerifyFindingsReportAGenuinelyMissingCredential(t *testing.T) {
	findings := verifyFindings(verifyFindingInputs{
		C2PAManifestFound:   true,
		C2PASignatureStatus: provenance.CheckValid,
		VCProofStatus:       provenance.CheckNotAvailable,
	})

	joined := strings.Join(findings, " | ")
	if !strings.Contains(joined, "Contract lifecycle credential missing from the PDF") {
		t.Errorf("a credential absent from a PDF pdf-core accepted must be reported: %s", joined)
	}
}

// jsonld_hash and base_pdf_hash on the contract-verify response were left nil
// because nothing in the repo computed a base-PDF hash from a re-render. pdf-core
// now reports both, so the optional fields carry them.
func TestContractVerifyDigestsAreCarriedFromPDFCore(t *testing.T) {
	result := pdfcore.VerifyResult{JSONLDHash: "1111", BasePDFHash: "2222"}

	jsonld, base := verifyDigests(result)

	if jsonld == nil || *jsonld != "1111" {
		t.Errorf("jsonld_hash: got %v, want 1111", jsonld)
	}
	if base == nil || *base != "2222" {
		t.Errorf("base_pdf_hash: got %v, want 2222", base)
	}
}

// A digest pdf-core could not compute stays absent. A pointer to "" would state
// that the hash is blank, which is a claim about the artifact rather than about
// the check — the exact confusion these optional fields exist to avoid.
func TestContractVerifyDigestsStayAbsentWhenPDFCoreComputedNone(t *testing.T) {
	jsonld, base := verifyDigests(pdfcore.VerifyResult{})

	if jsonld != nil {
		t.Errorf("jsonld_hash: got %q, want absent", *jsonld)
	}
	if base != nil {
		t.Errorf("base_pdf_hash: got %q, want absent", *base)
	}
}
