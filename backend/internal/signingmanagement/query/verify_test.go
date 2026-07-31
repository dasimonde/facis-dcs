package query

import (
	"testing"

	"digital-contracting-service/internal/pdfgeneration/pdfcore"
)

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
