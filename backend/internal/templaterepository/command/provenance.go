package command

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"digital-contracting-service/internal/pdfgeneration/provenance"
)

// TemplateProvenanceClaim is the per-version provenance assertion behind
// DCS-FR-TR-09: who created, reviewed, approved, and registered this
// template version, bound to the version's content hash and linked to the
// previous version's credential.
type TemplateProvenanceClaim struct {
	TemplateDID          string
	Version              int
	TemplateHash         string
	CreatedBy            string
	ReviewedBy           []string
	ApprovedBy           []string
	RegisteredBy         string
	RegistrarHolderDID   string
	PreviousCredentialID string
	EffectiveAt          time.Time
}

// templateProvenanceVC mirrors provenance.VCBinding's W3C VC 2.0 shape for
// the template-provenance credential type.
type templateProvenanceVC struct {
	Context           []interface{}          `json:"@context"`
	Type              []string               `json:"type"`
	ID                string                 `json:"id,omitempty"`
	Issuer            string                 `json:"issuer"`
	ValidFrom         string                 `json:"validFrom"`
	CredentialSubject map[string]interface{} `json:"credentialSubject"`
}

// IssueTemplateProvenanceVC builds and signs the W3C VC (JSON-LD, ecdsa-
// rdfc-2019 Data Integrity proof via the HSM VC key) for one registered
// template version. Same ID scheme as the contract lifecycle VCs
// (pdfgeneration/provenance/vc_binding.go): the credential ID is derived
// from the unsigned credential's hash.
func IssueTemplateProvenanceVC(ctx context.Context, signer provenance.VCSigner, issuerDID string, claim TemplateProvenanceClaim) (json.RawMessage, string, error) {
	subject := map[string]interface{}{
		"id":            "urn:dcs:template:" + claim.TemplateDID,
		"template_did":  claim.TemplateDID,
		"version":       claim.Version,
		"template_hash": claim.TemplateHash,
		"created_by":    claim.CreatedBy,
		"registered_by": claim.RegisteredBy,
	}
	if len(claim.ReviewedBy) > 0 {
		subject["reviewed_by"] = claim.ReviewedBy
	}
	if len(claim.ApprovedBy) > 0 {
		subject["approved_by"] = claim.ApprovedBy
	}
	if claim.RegistrarHolderDID != "" {
		subject["registrar_holder_did"] = claim.RegistrarHolderDID
	}
	// The versioning linkage DCS-FR-TR-09 demands: each credential names its
	// predecessor, so the whole version history is walkable and each link
	// individually verifiable.
	if claim.PreviousCredentialID != "" {
		subject["previous_credential"] = claim.PreviousCredentialID
	}

	unsigned := templateProvenanceVC{
		Context: []interface{}{
			"https://www.w3.org/ns/credentials/v2",
			"https://w3id.org/security/data-integrity/v2",
			map[string]interface{}{
				"dcs":                          "https://w3id.org/facis/dcs/ontology/v1#",
				"TemplateProvenanceCredential": "dcs:TemplateProvenanceCredential",
				"template_did":                 "dcs:templateDid",
				"version":                      "dcs:templateVersion",
				"template_hash":                "dcs:templateHash",
				"created_by":                   "dcs:createdBy",
				"reviewed_by":                  "dcs:reviewedBy",
				"approved_by":                  "dcs:approvedBy",
				"registered_by":                "dcs:registeredBy",
				"registrar_holder_did":         "dcs:registrarHolderDid",
				"previous_credential":          "dcs:previousCredential",
			},
		},
		Type:              []string{"VerifiableCredential", "TemplateProvenanceCredential"},
		Issuer:            issuerDID,
		ValidFrom:         claim.EffectiveAt.UTC().Format(time.RFC3339),
		CredentialSubject: subject,
	}

	raw, err := json.Marshal(unsigned)
	if err != nil {
		return nil, "", fmt.Errorf("marshal unsigned template provenance VC: %w", err)
	}
	h := sha256.Sum256(raw)
	vcID := "urn:dcs:vc:template-provenance:" + hex.EncodeToString(h[:])
	unsigned.ID = vcID

	rawWithID, err := json.Marshal(unsigned)
	if err != nil {
		return nil, "", fmt.Errorf("marshal template provenance VC with ID: %w", err)
	}

	signed, err := signer.CreateCredential(ctx, json.RawMessage(rawWithID))
	if err != nil {
		return nil, "", fmt.Errorf("sign template provenance VC: %w", err)
	}
	return signed, vcID, nil
}

// TemplateContentHash is the content binding used in provenance claims:
// sha256 over the stored template JSON-LD bytes.
func TemplateContentHash(templateData []byte) string {
	sum := sha256.Sum256(templateData)
	return hex.EncodeToString(sum[:])
}
