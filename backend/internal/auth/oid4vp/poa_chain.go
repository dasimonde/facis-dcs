package oid4vp

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"digital-contracting-service/internal/auth/oid4vp/sdjwt"
)

// MaxDelegationDepth is the largest number of delegation links a Power of
// Attorney may stand on above the credential an issuer trusted for the
// purpose issued (DCS-FR-SM-04).
const MaxDelegationDepth = 3

// DelegatedByClaim carries, in a delegated Power of Attorney, the delegator's
// own Power of Attorney: its issuer JWT and disclosures in compact SD-JWT form
// without a KB-JWT (<issuer-jwt>~<disclosure>~…~). The delegator signs the
// delegated credential with the key its own credential binds (cnf.jwk), and
// names itself as iss.
const DelegatedByClaim = "delegated_by"

// MayDelegateClaim set to true permits the holder of a Power of Attorney to
// delegate it. A credential without it cannot be the delegator of a link.
const MayDelegateClaim = "may_delegate"

// poaLink is one verified credential of a delegation chain.
type poaLink struct {
	document *verifiedDocument
	// statusClaims are what the status check of this link reads. A delegated
	// link's revocation entry is on the status list of the issuer that anchors
	// the chain, so its iss is that issuer.
	statusClaims json.RawMessage
}

// verifyPoAChain verifies a Power of Attorney and each delegation link above
// it, up to a credential an issuer trusted for purpose issued. It returns the
// links leaf first; the last one is the anchor. Each delegated link must be
// signed with the key the delegator's credential binds, name the delegator as
// iss, keep the delegator's organization, grant a subset of the delegator's
// roles, expire no later than the delegator's credential, and stand on a
// credential that permits delegation. Status is not checked here.
func verifyPoAChain(issuerJWT string, disclosures []string, trust *TrustConfig, purpose Purpose) ([]poaLink, error) {
	return walkPoAChain(issuerJWT, disclosures, trust, purpose, 0)
}

func walkPoAChain(issuerJWT string, disclosures []string, trust *TrustConfig, purpose Purpose, depth int) ([]poaLink, error) {
	parent, err := delegatedBy(issuerJWT)
	if err != nil {
		return nil, err
	}
	if parent == "" {
		document, err := verifyCredentialDocument(issuerJWT, disclosures, trust, purpose)
		if err != nil {
			return nil, err
		}
		return []poaLink{{document: document, statusClaims: document.RawClaims}}, nil
	}
	if depth >= MaxDelegationDepth {
		return nil, fmt.Errorf("power of attorney delegation chain is longer than %d links", MaxDelegationDepth)
	}

	parentJWT, parentDisclosures, err := splitCompactCredential(parent)
	if err != nil {
		return nil, fmt.Errorf("delegating power of attorney: %w", err)
	}
	chain, err := walkPoAChain(parentJWT, parentDisclosures, trust, purpose, depth+1)
	if err != nil {
		return nil, fmt.Errorf("delegating power of attorney: %w", err)
	}
	delegator := chain[0].document

	var delegatorClaims map[string]any
	if err := json.Unmarshal(delegator.RawClaims, &delegatorClaims); err != nil {
		return nil, fmt.Errorf("delegating power of attorney claims: %w", err)
	}
	if mayDelegate, _ := delegatorClaims[MayDelegateClaim].(bool); !mayDelegate {
		return nil, fmt.Errorf("the power of attorney of %q does not permit delegation", delegator.SubjectDID)
	}

	claims, err := sdjwt.VerifyDelegatedCredential(issuerJWT, disclosures, delegator.CNFJWK, trust.For(purpose))
	if err != nil {
		return nil, fmt.Errorf("delegated power of attorney: %w", err)
	}
	issuerID, _ := claims["iss"].(string)
	if issuerID != delegator.SubjectDID {
		return nil, fmt.Errorf("delegated power of attorney names issuer %q, not its delegator %q", issuerID, delegator.SubjectDID)
	}
	organization, err := sdjwt.OrganizationFromClaims(claims)
	if err != nil {
		return nil, err
	}
	if organization != delegator.Organization {
		return nil, fmt.Errorf("delegated power of attorney authorizes %q, its delegator only %q", organization, delegator.Organization)
	}
	roles, err := sdjwt.RolesFromClaims(claims)
	if err != nil {
		return nil, err
	}
	for _, role := range roles {
		if !slices.Contains(delegator.Roles, role) {
			return nil, fmt.Errorf("delegated power of attorney grants role %q, which its delegator does not hold", role)
		}
	}
	expires, err := claims.GetExpirationTime()
	if err != nil {
		return nil, err
	}
	delegatorExpires, err := jwt.MapClaims(delegatorClaims).GetExpirationTime()
	if err != nil {
		return nil, err
	}
	if delegatorExpires == nil || expires.After(delegatorExpires.Time) {
		return nil, fmt.Errorf("delegated power of attorney is valid after the power of attorney of its delegator expires")
	}
	cnfJWK, err := sdjwt.CNFJWKFromClaims(claims)
	if err != nil {
		return nil, fmt.Errorf("credential cnf.jwk: %w", err)
	}
	subject, _ := claims["sub"].(string)

	raw, err := json.Marshal(claims)
	if err != nil {
		return nil, err
	}
	anchor := chain[len(chain)-1].document.IssuerID
	statusClaims := jwt.MapClaims{}
	for name, value := range claims {
		statusClaims[name] = value
	}
	statusClaims["iss"] = anchor
	rawStatus, err := json.Marshal(statusClaims)
	if err != nil {
		return nil, err
	}

	link := poaLink{
		document: &verifiedDocument{
			IssuerID:     issuerID,
			SubjectDID:   subject,
			Organization: organization,
			Roles:        roles,
			CNFJWK:       cnfJWK,
			RawClaims:    raw,
		},
		statusClaims: rawStatus,
	}
	return append([]poaLink{link}, chain...), nil
}

// checkChainStatus refuses a chain any link of which is revoked, suspended or
// has no readable status.
func checkChainStatus(chain []poaLink) error {
	for _, link := range chain {
		if err := checkStatusList(link.statusClaims); err != nil {
			return fmt.Errorf("power of attorney of %q: %w", link.document.SubjectDID, err)
		}
	}
	return nil
}

// delegatedBy reads the delegated_by claim of an issuer JWT before its
// signature is verified: the claim names the credential that holds the key to
// verify it with. The signature check that follows covers the claim.
func delegatedBy(issuerJWT string) (string, error) {
	claims := jwt.MapClaims{}
	if _, _, err := jwt.NewParser().ParseUnverified(issuerJWT, claims); err != nil {
		return "", fmt.Errorf("credential jwt: %w", err)
	}
	raw, ok := claims[DelegatedByClaim]
	if !ok {
		return "", nil
	}
	parent, _ := raw.(string)
	if strings.TrimSpace(parent) == "" {
		return "", fmt.Errorf("credential claim %s must be a compact SD-JWT", DelegatedByClaim)
	}
	return strings.TrimSpace(parent), nil
}

// splitCompactCredential splits <issuer-jwt>~<disclosure>~…~ into its issuer
// JWT and disclosures. A trailing KB-JWT is refused: a delegator's credential
// is carried, not presented.
func splitCompactCredential(compact string) (string, []string, error) {
	parts := strings.Split(compact, "~")
	if len(parts) < 2 || parts[0] == "" || parts[len(parts)-1] != "" {
		return "", nil, fmt.Errorf("credential is not a compact SD-JWT without key binding")
	}
	disclosures := parts[1 : len(parts)-1]
	for i, disclosure := range disclosures {
		if disclosure == "" {
			return "", nil, fmt.Errorf("credential disclosure %d is empty", i+1)
		}
	}
	return parts[0], disclosures, nil
}
