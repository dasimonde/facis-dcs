package service

import (
	"database/sql"
	"testing"
	"time"

	processauditandcompliance "digital-contracting-service/gen/process_audit_and_compliance"
)

func TestSignatureEvidenceEntryUsesDurableSignatureMetadata(t *testing.T) {
	signedAt := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	entry := signatureEvidenceEntry(signatureEvidenceRecord{
		ID: 42, ContractDID: "did:example:contract", SignerDID: "did:example:signer",
		CredentialType: "NaturalPerson", Status: "SIGNED",
		SignedAt: sql.NullTime{Time: signedAt, Valid: true}, CreatedAt: signedAt,
		IPFSCID: sql.NullString{String: "bafy-signed-pdf", Valid: true},
		PDFHash: sql.NullString{String: "sha256:pdf", Valid: true},
	})
	if entry.EventType != "SIGNATURE_EVIDENCE" || entry.Did == nil || *entry.Did != "did:example:contract" {
		t.Fatalf("unexpected signature evidence envelope: %+v", entry)
	}
	data, ok := entry.EventData.(map[string]any)
	if !ok {
		t.Fatalf("signature evidence data has type %T", entry.EventData)
	}
	if data["signerDid"] != "did:example:signer" || data["status"] != "SIGNED" ||
		data["ipfsCid"] != "bafy-signed-pdf" || data["pdfHash"] != "sha256:pdf" {
		t.Fatalf("signature evidence lost durable metadata: %+v", data)
	}
	if _, exposesSignatureBytes := data["signatureBytes"]; exposesSignatureBytes {
		t.Fatal("signature evidence exposes signature bytes")
	}
	if _, duplicatesContractDID := data["contractDid"]; duplicatesContractDID {
		t.Fatal("signature evidence duplicates the resource DID in event data")
	}
}

func TestMergeAuditEvidenceResourcesCombinesSameComponentAndDID(t *testing.T) {
	baseEntry := signatureEvidenceEntry(signatureEvidenceRecord{
		ID: 1, ContractDID: "did:example:contract", CreatedAt: time.Now(),
	})
	addedEntry := signatureEvidenceEntry(signatureEvidenceRecord{
		ID: 2, ContractDID: "did:example:contract", CreatedAt: time.Now(),
	})
	base := []*auditEvidenceResource{{
		Did: "did:example:contract", Component: "Signature Management",
		AuditTrail: []*processauditandcompliance.PACResourceAuditTrailEntry{baseEntry},
	}}
	additions := []*auditEvidenceResource{{
		Did: "did:example:contract", Component: "Signature Management",
		AuditTrail: []*processauditandcompliance.PACResourceAuditTrailEntry{addedEntry},
	}}

	merged := mergeAuditEvidenceResources(base, additions)
	if len(merged) != 1 || len(merged[0].AuditTrail) != 2 {
		t.Fatalf("signature evidence was not merged: %+v", merged)
	}
}
