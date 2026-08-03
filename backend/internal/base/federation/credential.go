package federation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// dataIntegrityContext is the term namespace for DataIntegrityProof,
// cryptosuite and proofValue — the same suite ecdsa-rdfc-2019 uses for every
// other VC this codebase issues (see
// internal/pdfgeneration/provenance.HSMVCSigner).
const dataIntegrityContext = "https://w3id.org/security/data-integrity/v2"

// dcsVocabulary maps the agreement credential's own terms (not part of the
// W3C VC Data Model 2.0 base context) so RDFC-1.0 canonicalization — and
// therefore the ecdsa-rdfc-2019 signature — actually covers them, instead of
// silently dropping them as unmapped JSON-LD terms.
var dcsVocabulary = map[string]interface{}{
	"dcs":                           "https://w3id.org/facis/dcs/ontology/v1#",
	"FederationAgreementCredential": "dcs:FederationAgreementCredential",
	"TrustFrameworkPolicy":          "dcs:TrustFrameworkPolicy",
	"policyId": map[string]interface{}{
		"@id":   "dcs:policyId",
		"@type": "@id",
	},
	"hash": "dcs:rulesHash",
}

// Signer signs an unsigned VC (internal/pdfgeneration/provenance.VCSigner),
// kept as a local interface so this package does not depend on pdfgeneration.
type Signer interface {
	CreateCredential(ctx context.Context, unsignedVC json.RawMessage) (json.RawMessage, error)
}

const (
	// AgreementCredentialLifetime is how long an issued credential stays in
	// force. It is the window within which an instance removed from the
	// federation still presents a credential that verifies, so it is a day
	// rather than the process lifetime it used to be.
	AgreementCredentialLifetime = 24 * time.Hour

	// MaxAgreementCredentialLifetime is the longest window a PEER's credential
	// may claim. A validUntil far enough out is a credential that never expires,
	// which is the state requiring a validUntil at all exists to end; a peer on
	// another release cycle gets room beyond our own day either way.
	MaxAgreementCredentialLifetime = 7 * 24 * time.Hour
)

// BuildAgreementCredential builds and signs this instance's self-signed
// federation agreement credential (ADR-19): a W3C Verifiable Credential
// whose issuer is the instance's own DID, whose termsOfUse names the
// embedded federation rules by policyId (rulesURL) and hash (Hash()), and
// which is in force from validFrom for AgreementCredentialLifetime.
func BuildAgreementCredential(ctx context.Context, signer Signer, issuerDID, rulesURL string, validFrom time.Time) (json.RawMessage, error) {
	if signer == nil {
		return nil, fmt.Errorf("federation: no VC signer configured")
	}
	issuerDID = strings.TrimSpace(issuerDID)
	if issuerDID == "" {
		return nil, fmt.Errorf("federation: issuer DID is required")
	}
	rulesURL = strings.TrimSpace(rulesURL)
	if rulesURL == "" {
		return nil, fmt.Errorf("federation: federation rules policy URL is required")
	}

	validFrom = validFrom.UTC().Truncate(time.Second)
	unsigned := map[string]interface{}{
		"@context": []interface{}{
			"https://www.w3.org/ns/credentials/v2",
			dataIntegrityContext,
			dcsVocabulary,
		},
		"type":       []string{"VerifiableCredential", "FederationAgreementCredential"},
		"issuer":     issuerDID,
		"validFrom":  validFrom.Format(time.RFC3339),
		"validUntil": validFrom.Add(AgreementCredentialLifetime).Format(time.RFC3339),
		"credentialSubject": map[string]interface{}{
			"id": issuerDID,
		},
		"termsOfUse": map[string]interface{}{
			"type":     "TrustFrameworkPolicy",
			"policyId": rulesURL,
			"hash":     Hash(),
		},
	}

	raw, err := json.Marshal(unsigned)
	if err != nil {
		return nil, fmt.Errorf("marshal unsigned agreement credential: %w", err)
	}
	sum := sha256.Sum256(raw)
	unsigned["id"] = "urn:dcs:vc:" + hex.EncodeToString(sum[:])

	rawWithID, err := json.Marshal(unsigned)
	if err != nil {
		return nil, fmt.Errorf("marshal unsigned agreement credential with id: %w", err)
	}

	signed, err := signer.CreateCredential(ctx, json.RawMessage(rawWithID))
	if err != nil {
		return nil, fmt.Errorf("sign agreement credential: %w", err)
	}
	return signed, nil
}
