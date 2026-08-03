package sdjwt

import (
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// clockSkewLeeway is the tolerance applied to the credential's own time claims.
// Issuer, wallet and verifier clocks drift; keybinding.go allows the same for
// the KB-JWT, and a credential minted seconds ago on a marginally fast clock is
// not one that was issued in the future.
const clockSkewLeeway = 5 * time.Minute

// credentialSigningAlgs are the JWS algorithms an issuer may sign an SD-JWT VC
// with. ES256 is what EUDI mandates and what this deployment's own issuer uses;
// the others are here because real PID and QTSP issuer certificates are
// frequently RSA or a larger curve. What the key is allowed to be is decided by
// key resolution (keys.go), not by the algorithm name.
var credentialSigningAlgs = []string{
	"ES256", "ES384", "ES512",
	"PS256", "PS384", "PS512",
	"RS256", "RS384", "RS512",
}

// VerifyCredential validates the issuer JWT signature and returns the disclosed
// claims, with `sub` normalized to the holder this credential is bound to.
func VerifyCredential(token string, disclosures []string, cfg TrustConfig) (jwt.MapClaims, error) {
	if cfg == nil {
		return nil, fmt.Errorf("issuer trust is not configured")
	}

	parsed, err := jwt.NewParser(
		// A credential with no exp never expires. Requiring the claim is what
		// keeps a presentation bounded in time, so an issuer that omits it is a
		// bug at the issuer, not a rule to relax here.
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithLeeway(clockSkewLeeway),
		jwt.WithValidMethods(credentialSigningAlgs),
	).Parse(token, func(t *jwt.Token) (any, error) {
		return ResolveIssuerVerificationKey(cfg, t)
	})

	if err != nil {
		return nil, fmt.Errorf("credential jwt: %w", err)
	}

	err = validateCredentialHeader(parsed)
	if err != nil {
		return nil, err
	}

	issuerClaims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("credential jwt claims are invalid")
	}

	vct, _ := issuerClaims["vct"].(string)
	if !cfg.VCTAllowed(strings.TrimSpace(vct)) {
		return nil, fmt.Errorf("vct %q is not allowed", vct)
	}

	merged, err := MergeDisclosedClaims(issuerClaims, disclosures)
	if err != nil {
		return nil, err
	}

	sub, err := HolderSubject(merged)
	if err != nil {
		return nil, err
	}
	merged["sub"] = sub

	return merged, nil
}
