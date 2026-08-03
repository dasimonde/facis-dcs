package provenance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// SigningSummary carries the fields bound into a ContractSigningSummaryCredential
// (DCS-FR-SM-08). It records who signed (the signer's holder DID — pseudonymous),
// through which ceremony, and the hashes the PAdES signature covers. The signer's
// PID is NOT embedded: the disclosed Person Identification Data (name, birthdate,
// address) never enters the shared PDF (eIDAS/GDPR data-minimisation). Only the
// pseudonymous holder DID and the KB-JWT sd_hash of the presented credential are
// bound in — enough to cryptographically tie the signature to the credential the
// internal ceremony record holds, without leaking personal data downstream.
type SigningSummary struct {
	ContractID     string
	SignerDID      string
	CeremonyID     string
	FieldName      string
	ContentHash    string // SHA-256 hex of the JSON-LD contract source
	PDFHash        string // SHA-256 hex of the base PDF bytes bound by the signature
	CredentialType string // e.g. "AES"
	KBSDHash       string // KB-JWT sd_hash binding the signature to the presented credential
	SignedAt       time.Time
	// SchemaVersion/ValidationReportHash (Phase 4, ADR-9): the Semantic Hub
	// SHACL shapes version this contract validated against at signing time,
	// and a stable hash of the resulting findings
	// (validation.SHACLEvidence) — an external verifier resolves
	// sh:shapesGraph to fetch those exact pinned shapes from the public hub
	// endpoints, re-runs validation, and compares hashes to detect drift.
	// Optional: zero/empty when SHACL evidence could not be produced (e.g.
	// signing evidence is mandatory).
	SchemaVersion        int
	ValidationReportHash string
}

// IssueSigningSummaryVC builds and signs a ContractSigningSummaryCredential over
// the given summary, using the same VC signer as the lifecycle credentials. It
// binds the pseudonymous holder DID and the KB-JWT sd_hash — never the PID
// disclosures — so the signed PDF carries no personal data (DCS-FR-SM-08,
// UC-04-03).
//
// status is the CONTRACT's entry in the status list this deployment serves — the
// same entry its lifecycle credentials name (ADR-34). This credential travels
// beside the contract to the counterparty and outlives the ceremony that
// produced it, so it has to be revocable, and revocable by the one thing that
// ever revokes here: the contract falling out of force.
func IssueSigningSummaryVC(ctx context.Context, signer VCSigner, issuerDID string, status CredentialStatusRef, s SigningSummary) (json.RawMessage, string, error) {
	unsignedVC := VCBinding{
		Context: []interface{}{
			"https://www.w3.org/ns/credentials/v2",
			dataIntegrityContext,
			map[string]interface{}{
				"dcs":                              "https://w3id.org/facis/dcs/ontology/v1#",
				"ContractSigningSummaryCredential": "dcs:ContractSigningSummaryCredential",
				"contract_id":                      "dcs:contractId",
				"ceremony_id":                      "dcs:ceremonyId",
				"field_name":                       "dcs:fieldName",
				"content_hash":                     "dcs:contentHash",
				"pdf_hash":                         "dcs:pdfHash",
				"credential_type":                  "dcs:credentialType",
				"kb_sd_hash":                       "dcs:kbSdHash",
				"signed_at": map[string]interface{}{
					"@id":   "dcs:signedAt",
					"@type": "http://www.w3.org/2001/XMLSchema#dateTime",
				},
				"schema_version":         "dcs:schemaVersion",
				"validation_report_hash": "dcs:validationReportHash",
			},
		},
		Type:             []string{"VerifiableCredential", "ContractSigningSummaryCredential"},
		Issuer:           issuerDID,
		ValidFrom:        s.SignedAt.UTC().Format(time.RFC3339),
		CredentialStatus: buildCredentialStatus(status),
		CredentialSubject: map[string]interface{}{
			"id":           normalizeSubjectID(s.SignerDID),
			"contract_id":  s.ContractID,
			"ceremony_id":  s.CeremonyID,
			"field_name":   s.FieldName,
			"content_hash": s.ContentHash,
			"pdf_hash":     s.PDFHash,
			"kb_sd_hash":   s.KBSDHash,
			"signed_at":    s.SignedAt.UTC().Format(time.RFC3339),
		},
	}
	// A field whose signatory has not signed yet has no established level, so
	// the caller leaves CredentialType empty. Emitting "" would seal an empty
	// eIDAS level claim into an issuer-signed credential inside a PAdES byte
	// range that can never be corrected; omitting the key states the absence.
	if s.CredentialType != "" {
		unsignedVC.CredentialSubject["credential_type"] = s.CredentialType
	}
	if s.ValidationReportHash != "" {
		unsignedVC.CredentialSubject["schema_version"] = s.SchemaVersion
		unsignedVC.CredentialSubject["validation_report_hash"] = s.ValidationReportHash
	}

	raw, err := json.Marshal(unsignedVC)
	if err != nil {
		return nil, "", fmt.Errorf("marshal unsigned signing-summary VC: %w", err)
	}
	h := sha256.Sum256(raw)
	vcID := "urn:dcs:vc:" + hex.EncodeToString(h[:])
	unsignedVC.ID = vcID

	rawWithID, err := json.Marshal(unsignedVC)
	if err != nil {
		return nil, "", fmt.Errorf("marshal unsigned signing-summary VC with ID: %w", err)
	}

	signed, err := signer.CreateCredential(ctx, json.RawMessage(rawWithID))
	if err != nil {
		return nil, "", fmt.Errorf("sign signing-summary VC: %w", err)
	}
	return signed, vcID, nil
}
