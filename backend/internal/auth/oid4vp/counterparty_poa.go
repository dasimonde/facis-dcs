package oid4vp

import (
	"encoding/json"
	"fmt"
	"strings"

	"digital-contracting-service/internal/auth/oid4vp/sdjwt"
)

// CounterpartyPoA is a counterparty's Power of Attorney as this instance
// verified it: who issued it, which organization it authorizes, and which
// holder it is bound to.
type CounterpartyPoA struct {
	IssuerID     string
	SignatoryDID string
	Organization string
	Roles        []string
	RawClaims    json.RawMessage
}

// CounterpartyPoAExpectation is what the receiving instance already knows about
// the signature the credential is supposed to authorize, read from the contract
// the peer shipped: the party that signed, and the signatory recorded for it.
// The credential has to match both, or it authorizes some other signature.
type CounterpartyPoAExpectation struct {
	Organization string
	SignatoryDID string
}

// VerifyCounterpartyPoA verifies a Power of Attorney a counterparty presented
// on its own instance and shipped along with the signed contract.
//
// It is the same credential verification a signing ceremony runs, minus the one
// part that cannot travel: the KB-JWT proves the holder answered a specific
// request, with a nonce and an audience issued by the verifier that asked. This
// instance never asked, so it re-derives nothing from that segment — the holder
// binding it checks is the credential's own (sub against cnf.jwk) plus the
// requirement that this holder is the signatory the shipped contract records.
//
// What that establishes is an attestation: an issuer this instance trusts for
// `peer`, entitled to speak for that organization, says the holder may act for
// it, and the credential is not revoked. It does not establish who the human
// is — the PID does that, on the instance where the ceremony ran.
func VerifyCounterpartyPoA(presentation string, trust *TrustConfig, expected CounterpartyPoAExpectation) (*CounterpartyPoA, error) {
	if trust == nil {
		return nil, fmt.Errorf("trust config is not configured")
	}

	organization := strings.TrimSpace(expected.Organization)
	signatory := strings.TrimSpace(expected.SignatoryDID)
	if organization == "" {
		return nil, fmt.Errorf("no party to verify the Power of Attorney against")
	}
	if signatory == "" {
		return nil, fmt.Errorf("party %q records no signatory to bind the Power of Attorney to", organization)
	}

	parsed, err := sdjwt.ParsePresentation(strings.TrimSpace(presentation))
	if err != nil {
		return nil, err
	}

	document, err := verifyCredentialDocument(parsed.IssuerJWT, parsed.Disclosures, trust, PurposePeer)
	if err != nil {
		return nil, err
	}

	if err := checkStatusList(document.RawClaims); err != nil {
		return nil, err
	}

	if document.Organization != organization {
		return nil, fmt.Errorf("power of attorney authorizes %q, not the signing party %q", document.Organization, organization)
	}
	if document.SubjectDID != signatory {
		return nil, fmt.Errorf("power of attorney is held by %q, not by %q, the signatory recorded for party %q",
			document.SubjectDID, signatory, organization)
	}

	return &CounterpartyPoA{
		IssuerID:     document.IssuerID,
		SignatoryDID: document.SubjectDID,
		Organization: document.Organization,
		Roles:        document.Roles,
		RawClaims:    document.RawClaims,
	}, nil
}
