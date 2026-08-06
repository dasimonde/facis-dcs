package provenance

import (
	"encoding/json"
	"fmt"
	"strings"
)

// SigningEvidenceAttachment is one PDF/A-3 associated file embedded before a
// signature is applied: everything a holder of the PDF needs to judge that ONE
// signing event. The signing instance embeds its own before its own PAdES
// signature, so the signature covers the authorization behind it and the
// counterparty verifies both out of the PDF (ADR-13, ADR-31, ADR-35).
type SigningEvidenceAttachment struct {
	// Summary is the ContractSigningSummaryCredential the signing instance
	// issued for the ceremony (DCS-FR-SM-08). It names the field, the
	// signatory and the contract, and is issuer-signed, so the binding it
	// asserts is checkable rather than a claim beside the credential.
	Summary json.RawMessage `json:"summary"`
	// PoAPresentation is the Power of Attorney verbatim as the signatory's
	// wallet delivered it at the ceremony. Empty when the ceremony had none.
	PoAPresentation string `json:"poa_presentation,omitempty"`
}

// SigningEvidenceSubject is what a summary credential attests about the
// signature it stands behind: the field it was made for, the signatory that
// made it, and the contract it belongs to.
type SigningEvidenceSubject struct {
	Signatory  string
	FieldName  string
	ContractID string
}

// ReadSigningEvidence decodes one extracted attachment and the subject of the
// summary it carries. A document that is not a signing summary is refused here
// rather than further down, where its empty fields would read as an unattested
// signature.
func ReadSigningEvidence(raw json.RawMessage) (SigningEvidenceAttachment, SigningEvidenceSubject, error) {
	var attachment SigningEvidenceAttachment
	if err := json.Unmarshal(raw, &attachment); err != nil {
		return SigningEvidenceAttachment{}, SigningEvidenceSubject{}, fmt.Errorf("decode signing evidence attachment: %w", err)
	}
	if len(attachment.Summary) == 0 {
		return SigningEvidenceAttachment{}, SigningEvidenceSubject{}, fmt.Errorf("signing evidence attachment carries no signing summary")
	}
	subject, err := ReadSigningEvidenceSubject(attachment.Summary)
	if err != nil {
		return SigningEvidenceAttachment{}, SigningEvidenceSubject{}, err
	}
	return attachment, subject, nil
}

// ReadSigningEvidenceSubject reads the attested signature out of a
// ContractSigningSummaryCredential.
func ReadSigningEvidenceSubject(summary json.RawMessage) (SigningEvidenceSubject, error) {
	var vc struct {
		Type              []string `json:"type"`
		CredentialSubject struct {
			ID         string `json:"id"`
			FieldName  string `json:"field_name"`
			ContractID string `json:"contract_id"`
		} `json:"credentialSubject"`
	}
	if err := json.Unmarshal(summary, &vc); err != nil {
		return SigningEvidenceSubject{}, fmt.Errorf("decode signing summary: %w", err)
	}
	isSummary := false
	for _, t := range vc.Type {
		if t == "ContractSigningSummaryCredential" {
			isSummary = true
			break
		}
	}
	if !isSummary {
		return SigningEvidenceSubject{}, fmt.Errorf("the embedded evidence is not a signing summary")
	}
	return SigningEvidenceSubject{
		Signatory:  strings.TrimSpace(vc.CredentialSubject.ID),
		FieldName:  strings.TrimSpace(vc.CredentialSubject.FieldName),
		ContractID: strings.TrimSpace(vc.CredentialSubject.ContractID),
	}, nil
}
