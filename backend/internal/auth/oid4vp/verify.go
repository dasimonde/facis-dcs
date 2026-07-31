package oid4vp

import (
	"encoding/json"
	"fmt"

	"digital-contracting-service/internal/auth/oid4vp/sdjwt"
	"digital-contracting-service/internal/base/datatype/userrole"
)

// PresentationContext carries OpenID4VP request parameters required for verification.
type PresentationContext struct {
	Nonce    string
	ClientID string
}

// VerifiedLoginClaims holds subject and roles extracted from a verified VP.
type VerifiedLoginClaims struct {
	IssuerID       string
	SubjectDID     string
	ParticipantDID string
	Roles          []string
	GrantedRoles   []string
	RawClaims      json.RawMessage
}

// VerifiedPIDClaims holds subject data extracted from a verified PID presentation.
type VerifiedPIDClaims struct {
	SubjectDID string
	RawClaims  json.RawMessage
}

// Verifier validates a wallet presentation and returns login claims.
type Verifier interface {
	Verify(vpToken string, ctx PresentationContext) (*VerifiedLoginClaims, error)
	VerifyPID(vpToken string, ctx PresentationContext) (*VerifiedPIDClaims, error)
}

// NewVerifier returns a VP verifier backed by the given issuer trust configuration.
// NewVerifier builds a verifier restricted to one purpose. The purpose decides
// which issuers are acceptable at all: an issuer granted `peer` verifies a
// counterparty's PoA but cannot mint a session here (ADR-31).
func NewVerifier(cfg *TrustConfig, purpose Purpose) Verifier {
	if cfg == nil {
		return unconfiguredVerifier{}
	}
	return verifier{trust: cfg, purpose: purpose}
}

type verifier struct {
	trust   *TrustConfig
	purpose Purpose
}

type unconfiguredVerifier struct{}

func (unconfiguredVerifier) Verify(_ string, _ PresentationContext) (*VerifiedLoginClaims, error) {
	return nil, fmt.Errorf("oid4vp trust config is not loaded (set OID4VP_TRUST_DATA_PATH)")
}

func (unconfiguredVerifier) VerifyPID(_ string, _ PresentationContext) (*VerifiedPIDClaims, error) {
	return nil, fmt.Errorf("oid4vp trust config is not loaded (set OID4VP_TRUST_DATA_PATH)")
}

func (v verifier) Verify(vpToken string, ctx PresentationContext) (*VerifiedLoginClaims, error) {
	// Policy verification steps, in order of execution:
	// 1. trust list + wallet binding (parse VP, issuer sig, trust, cnf/sub, KB sig + aud/nonce/sd_hash)
	// 2. status list
	// 3. login roles
	verified, err := verifyTrustAndWallet(vpToken, ctx, v.trust, v.purpose)
	if err != nil {
		return nil, err
	}

	err = checkStatusList(verified.RawClaims)
	if err != nil {
		return nil, err
	}

	granted, err := evaluateLoginRoles(verified.Roles)
	if err != nil {
		return nil, err
	}
	verified.GrantedRoles = granted

	return verified, nil
}

func (v verifier) VerifyPID(vpToken string, ctx PresentationContext) (*VerifiedPIDClaims, error) {
	verified, err := verifyTrustAndWalletForPID(vpToken, ctx, v.trust, v.purpose)
	if err != nil {
		return nil, err
	}

	// Self-issued dev PIDs carry a real status claim (ensure_statuslist_for_dev.py,
	// mirroring the PoA credential's status wiring), so this now runs for real
	// instead of vacuously (ADR-20 — EUDIPLO, which omitted status, is removed).
	if err := checkStatusList(verified.RawClaims); err != nil {
		return nil, err
	}

	return verified, nil
}

func verifyTrustAndWalletForPID(vpToken string, ctx PresentationContext, trust *TrustConfig, purpose Purpose) (*VerifiedPIDClaims, error) {
	if trust == nil {
		return nil, fmt.Errorf("trust config is not configured")
	}

	presentation, err := sdjwt.ParsePresentation(vpToken)
	if err != nil {
		return nil, err
	}

	issuerClaims, err := sdjwt.VerifyCredential(presentation.IssuerJWT, presentation.Disclosures, trust.For(purpose))
	if err != nil {
		return nil, err
	}

	cnfJWK, err := sdjwt.CNFJWKFromClaims(issuerClaims)
	if err != nil {
		return nil, fmt.Errorf("credential cnf.jwk: %w", err)
	}

	sub, _ := issuerClaims["sub"].(string)

	err = sdjwt.VerifyKB(presentation.KBJWT, presentation.SDHash, cnfJWK, sub, ctx.Nonce, ctx.ClientID)
	if err != nil {
		return nil, err
	}

	raw, err := json.Marshal(issuerClaims)
	if err != nil {
		return nil, err
	}

	return &VerifiedPIDClaims{
		SubjectDID: sub,
		RawClaims:  raw,
	}, nil
}

// verifiedDocument is a dc+sd-jwt credential verified as a DOCUMENT: issuer
// signature, issuer trust for the purpose at hand, vct, validity window,
// disclosure integrity, the holder binding between sub and cnf.jwk, and the
// issuer's entitlement to name the organization it names.
//
// What it deliberately does not cover is the KB-JWT, the only part of a
// presentation that says its holder is here NOW, answering THIS request: that
// binds to a nonce and an audience only the verifier that issued them can
// check.
type verifiedDocument struct {
	IssuerID     string
	SubjectDID   string
	Organization string
	Roles        []string
	CNFJWK       sdjwt.JWK
	RawClaims    json.RawMessage
}

func verifyCredentialDocument(issuerJWT string, disclosures []string, trust *TrustConfig, purpose Purpose) (*verifiedDocument, error) {
	if trust == nil {
		return nil, fmt.Errorf("trust config is not configured")
	}

	// Verify issuer signature, header.jwk trust, vct/exp/iat, merge disclosures,
	// resolve the holder the credential is bound to (sub, or the did:jwk of
	// cnf.jwk when the issuer named no subject).
	issuerClaims, err := sdjwt.VerifyCredential(issuerJWT, disclosures, trust.For(purpose))
	if err != nil {
		return nil, err
	}

	// Holder binding: cnf.jwk is the verification key the KB-JWT must be signed with.
	cnfJWK, err := sdjwt.CNFJWKFromClaims(issuerClaims)
	if err != nil {
		return nil, fmt.Errorf("credential cnf.jwk: %w", err)
	}

	sub, _ := issuerClaims["sub"].(string)

	roles, err := sdjwt.RolesFromClaims(issuerClaims)
	if err != nil {
		return nil, err
	}

	organization, err := sdjwt.OrganizationFromClaims(issuerClaims)
	if err != nil {
		return nil, err
	}

	// An issuer may only speak for the organizations its trust entry names.
	// Without this the verifier would rely on every trusted issuer being
	// well-behaved: a counterparty's issuer could assert this instance's
	// organization and any organization check downstream would pass.
	issuerID, _ := issuerClaims["iss"].(string)
	if !trust.For(purpose).IssuerMayAttest(issuerID, organization) {
		return nil, fmt.Errorf("issuer %q is not entitled to attest organization %q", issuerID, organization)
	}

	raw, err := json.Marshal(issuerClaims)
	if err != nil {
		return nil, err
	}

	return &verifiedDocument{
		IssuerID:     issuerID,
		SubjectDID:   sub,
		Organization: organization,
		Roles:        roles,
		CNFJWK:       cnfJWK,
		RawClaims:    raw,
	}, nil
}

func verifyTrustAndWallet(vpToken string, ctx PresentationContext, trust *TrustConfig, purpose Purpose) (*VerifiedLoginClaims, error) {
	if trust == nil {
		return nil, fmt.Errorf("trust config is not configured")
	}

	// Parse SD-JWT~disclosures~KB-JWT presentation.
	presentation, err := sdjwt.ParsePresentation(vpToken)
	if err != nil {
		return nil, err
	}

	document, err := verifyCredentialDocument(presentation.IssuerJWT, presentation.Disclosures, trust, purpose)
	if err != nil {
		return nil, err
	}

	// KB-JWT: signature via cnf.jwk; payload aud, nonce, sd_hash.
	err = sdjwt.VerifyKB(presentation.KBJWT, presentation.SDHash, document.CNFJWK, document.SubjectDID, ctx.Nonce, ctx.ClientID)
	if err != nil {
		return nil, err
	}

	return &VerifiedLoginClaims{
		IssuerID:       document.IssuerID,
		SubjectDID:     document.SubjectDID,
		ParticipantDID: document.Organization,
		Roles:          document.Roles,
		RawClaims:      document.RawClaims,
	}, nil
}

// evaluateLoginRoles applies login authorization policy to disclosed roles.
func evaluateLoginRoles(disclosedRoles []string) ([]string, error) {
	if len(disclosedRoles) == 0 {
		return nil, fmt.Errorf("no roles disclosed in presentation")
	}

	granted := make([]string, 0, len(disclosedRoles))
	for _, role := range disclosedRoles {
		ur, err := userrole.NewUserRole(role)
		if err != nil {
			return nil, fmt.Errorf("invalid disclosed role %q: %w", role, err)
		}
		granted = append(granted, ur.String())
	}

	return granted, nil
}
