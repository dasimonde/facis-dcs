package service

import (
	"testing"

	"digital-contracting-service/internal/signingmanagement/query"
)

// The verify endpoint previously discarded the verifier's result and returned a
// zero value, so a revoked contract and a sound one encoded identically. Every
// verdict the verifier computes has to reach the response.
func TestVerifyResponseCarriesTheVerifiersVerdict(t *testing.T) {
	jsonldHash := "a1b2"
	basePdfHash := "c3d4"
	result := &query.SignatureVerifyResult{
		Match:       true,
		SigCount:    2,
		Findings:    []string{"C2PA signature invalid: leaf certificate has no O=", "Status list state: REVOKED"},
		JsonldHash:  &jsonldHash,
		BasePdfHash: &basePdfHash,
	}

	res := verifyResponseFrom("did:web:dcs.example:contract:1", result)

	if res.Did != "did:web:dcs.example:contract:1" {
		t.Errorf("did: got %q", res.Did)
	}
	if !res.Match {
		t.Error("match: got false, want true")
	}
	if res.SigCount != 2 {
		t.Errorf("sig_count: got %d, want 2", res.SigCount)
	}
	if len(res.Findings) != 2 {
		t.Fatalf("findings: got %v", res.Findings)
	}
	for i, want := range result.Findings {
		if res.Findings[i] != want {
			t.Errorf("findings[%d]: got %q, want %q", i, res.Findings[i], want)
		}
	}
	if res.JsonldHash == nil || *res.JsonldHash != jsonldHash {
		t.Errorf("jsonld_hash: got %v", res.JsonldHash)
	}
	if res.BasePdfHash == nil || *res.BasePdfHash != basePdfHash {
		t.Errorf("base_pdf_hash: got %v", res.BasePdfHash)
	}
}

// A failing verification must be distinguishable from the zero value: match
// false with the reason attached, not match false alone.
func TestVerifyResponseReportsAFailedVerificationWithItsFindings(t *testing.T) {
	result := &query.SignatureVerifyResult{
		Match:    false,
		SigCount: 1,
		Findings: []string{"C2PA manifest not found"},
	}

	res := verifyResponseFrom("did:web:dcs.example:contract:2", result)

	if res.Match {
		t.Error("match: got true, want false")
	}
	if res.SigCount != 1 {
		t.Errorf("sig_count: got %d, want 1", res.SigCount)
	}
	if len(res.Findings) != 1 || res.Findings[0] != "C2PA manifest not found" {
		t.Errorf("findings: got %v", res.Findings)
	}
}
