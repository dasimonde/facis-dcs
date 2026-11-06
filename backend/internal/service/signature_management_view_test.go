package service

import (
	"testing"

	signaturemanagement "digital-contracting-service/gen/signature_management"
	"digital-contracting-service/internal/signingmanagement/db"
	"digital-contracting-service/internal/signingmanagement/query"
)

// Each Compliance Viewer row shows the credential status of its own signature.
// A row without matching evidence has nothing to look up, so it is UNKNOWN
// with a reason and not the state of another signer's credential.
func TestEnrichWithSigningEvidenceSetsTheCredentialStatusPerSignature(t *testing.T) {
	evidence := []query.SigningEvidence{{
		SignerDID:        "did:example:revoked-signer",
		CredentialStatus: query.CredentialStatus{State: query.CredentialRevoked},
	}}

	matched := &signaturemanagement.SMSignatureViewItem{}
	enrichWithSigningEvidence(matched, db.SignatureRecord{SignerDID: "did:example:revoked-signer"}, evidence)
	if matched.CredentialStatus != query.CredentialRevoked || matched.CredentialStatusReason != nil {
		t.Fatalf("expected REVOKED without a reason, got %q %v", matched.CredentialStatus, matched.CredentialStatusReason)
	}

	unmatched := &signaturemanagement.SMSignatureViewItem{}
	enrichWithSigningEvidence(unmatched, db.SignatureRecord{SignerDID: "did:example:other-signer"}, evidence)
	if unmatched.CredentialStatus != query.CredentialUnknown || unmatched.CredentialStatusReason == nil {
		t.Fatalf("expected UNKNOWN with a reason, got %q %v", unmatched.CredentialStatus, unmatched.CredentialStatusReason)
	}
}
