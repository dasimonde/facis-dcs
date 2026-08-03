package provenance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// VCSigner signs unsigned VCs via the Crypto Provider Service.
type VCSigner interface {
	CreateCredential(ctx context.Context, unsignedVC json.RawMessage) (json.RawMessage, error)
}

// VCBinding is the W3C VC issued to bind a lifecycle event to contract_id + file_hash
// (DCS-OR-C2PA-004). It is returned by IssueLifecycleVC and stored as vc_id in the
// LifecycleAssertion once issued. Uses W3C VC Data Model 2.0 field names.
type VCBinding struct {
	Context           []interface{}          `json:"@context"`
	Type              []string               `json:"type"`
	ID                string                 `json:"id"`
	Issuer            string                 `json:"issuer"`
	ValidFrom         string                 `json:"validFrom"`
	CredentialSubject map[string]interface{} `json:"credentialSubject"`
	// CredentialStatus links this VC to its entry in the status list this
	// deployment serves, so verifiers can check revocation
	// (DCS-OR-C2PA-004, DCS-OR-C2PA-005, ADR-34).
	CredentialStatus map[string]interface{} `json:"credentialStatus,omitempty"`
}

// IssueLifecycleVC builds and signs a W3C VC recording the lifecycle event.
// status is the contract's allocated entry in the status list this deployment
// serves; it is embedded as credentialStatus so verifiers can check revocation
// (DCS-OR-C2PA-004, DCS-OR-C2PA-005). It is passed in rather than derived here
// so the entry a credential advertises is by construction the entry the
// publisher allocated and will later flip.
// The signed VC bytes are returned; the VC id is derived from the SHA-256 of
// its content so it can be stored in LifecycleAssertion.VCId.
func IssueLifecycleVC(ctx context.Context, signer VCSigner, issuerDID string, status CredentialStatusRef, assertion LifecycleAssertion) (json.RawMessage, string, error) {
	subjectID := normalizeSubjectID(assertion.ContractID)
	securityCtx := vcSecuritySuiteContext()

	unsignedVC := VCBinding{
		Context: []interface{}{
			"https://www.w3.org/ns/credentials/v2",
			securityCtx,
			map[string]interface{}{
				"dcs":                         "https://w3id.org/facis/dcs/ontology/v1#",
				"ContractLifecycleCredential": "dcs:ContractLifecycleCredential",
				"contract_id":                 "dcs:contractId",
				"file_hash":                   "dcs:fileHash",
				"status":                      "dcs:status",
				"reason":                      "dcs:reason",
				"effective_at": map[string]interface{}{
					"@id":   "dcs:effectiveAt",
					"@type": "http://www.w3.org/2001/XMLSchema#dateTime",
				},
			},
		},
		Type:      []string{"VerifiableCredential", "ContractLifecycleCredential"},
		ID:        "", // filled after hash
		Issuer:    issuerDID,
		ValidFrom: assertion.EffectiveAt.UTC().Format(time.RFC3339),
		CredentialSubject: map[string]interface{}{
			"id":           subjectID,
			"contract_id":  assertion.ContractID,
			"file_hash":    assertion.FileHash,
			"status":       assertion.Status,
			"reason":       assertion.Reason,
			"effective_at": assertion.EffectiveAt.UTC().Format(time.RFC3339),
		},
		CredentialStatus: buildCredentialStatus(status),
	}

	raw, err := json.Marshal(unsignedVC)
	if err != nil {
		return nil, "", fmt.Errorf("marshal unsigned VC: %w", err)
	}

	h := sha256.Sum256(raw)
	vcID := "urn:dcs:vc:" + hex.EncodeToString(h[:])
	unsignedVC.ID = vcID

	rawWithID, err := json.Marshal(unsignedVC)
	if err != nil {
		return nil, "", fmt.Errorf("marshal unsigned VC with ID: %w", err)
	}

	signed, err := signer.CreateCredential(ctx, json.RawMessage(rawWithID))
	if err != nil {
		return nil, "", fmt.Errorf("sign lifecycle VC: %w", err)
	}

	return signed, vcID, nil
}

// buildCredentialStatus constructs the credentialStatus object that links a VC
// to its entry in the status list this deployment serves (DCS-OR-C2PA-005,
// ADR-34). Returns nil when the status list credential is empty so the field is
// omitted from the VC.
//
// The entry belongs to the CONTRACT, so every lifecycle credential a
// contract accumulates — the draft, the active one the signature commits to,
// the terminated one, and the signing summaries beside them — advertises the
// same index and one revocation invalidates all of them. That is the scope on
// purpose: the bit says whether the contract is still in force, the only thing
// that ever sets it is a terminal contract state
// (DCSStatusListPublisher.PublishStatus), and no per-credential revoke exists to
// set anything else. Per-credential indices would leave a terminated contract's
// superseded draft credential reading not-revoked forever, and a reader holding
// only that credential would conclude the contract is live.
//
// The type names a token status list because that is what the URI serves: a
// signed statuslist+jwt whose status_list.lst is a zlib-compressed, LSB-first
// bitstring. Declaring the W3C BitstringStatusListEntry would tell a verifier to
// expect a BitstringStatusListCredential with credentialSubject.encodedList and
// MSB-first bits — a document that is not there, and a bit order that reads a
// different contract's entry where it is.
func buildCredentialStatus(status CredentialStatusRef) map[string]interface{} {
	if status.StatusListCredential == "" {
		return nil
	}
	return map[string]interface{}{
		"id":                   fmt.Sprintf("%s#%d", status.StatusListCredential, status.Index),
		"type":                 statusListEntryType,
		"statusPurpose":        "revocation",
		"statusListIndex":      fmt.Sprintf("%d", status.Index),
		"statusListCredential": status.StatusListCredential,
	}
}

func vcSecuritySuiteContext() string {
	return dataIntegrityContext
}

// normalizeSubjectID returns a URI to satisfy strict VC signer validation.
// If the input is already an absolute URI (including did:... and urn:...), it
// is used as-is. Otherwise, a deterministic URN is generated from the raw value.
func normalizeSubjectID(raw string) string {
	s := strings.TrimSpace(raw)
	if s != "" {
		u, err := url.Parse(s)
		if err == nil && u.IsAbs() && u.Scheme != "" {
			return s
		}
	}
	h := sha256.Sum256([]byte(s))
	return "urn:dcs:subject:" + hex.EncodeToString(h[:])
}
