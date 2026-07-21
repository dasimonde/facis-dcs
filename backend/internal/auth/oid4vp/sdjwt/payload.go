package sdjwt

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// VerifyCredential validates the issuer JWT signature and returns merged disclosed claims.
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
		jwt.WithValidMethods([]string{"ES256"}),
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

	sub, _ := issuerClaims["sub"].(string)
	if strings.TrimSpace(sub) == "" {
		return nil, fmt.Errorf("credential jwt missing sub")
	}

	vct, _ := issuerClaims["vct"].(string)
	if !cfg.VCTAllowed(strings.TrimSpace(vct)) {
		return nil, fmt.Errorf("vct %q is not allowed", vct)
	}

	err = validateNotBeforeIfPresent(issuerClaims)
	if err != nil {
		return nil, err
	}

	err = VerifyDisclosures(issuerClaims, disclosures)
	if err != nil {
		return nil, err
	}

	return MergeDisclosedClaims(issuerClaims, disclosures)
}

// VerifyCredentialForPID validates PID issuer JWTs, including playground credentials
// that sign with x5c.
func VerifyCredentialForPID(token string, disclosures []string, cfg TrustConfig) (jwt.MapClaims, error) {
	if cfg == nil {
		return nil, fmt.Errorf("issuer trust is not configured")
	}

	parsed, err := jwt.NewParser(
		// A credential with no exp never expires. Requiring the claim is what
		// keeps a presentation bounded in time, so an issuer that omits it is a
		// bug at the issuer, not a rule to relax here.
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithValidMethods([]string{"ES256"}),
	).Parse(token, func(t *jwt.Token) (any, error) {
		return ResolveIssuerVerificationKeyForPID(t)
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

	err = validateNotBeforeIfPresent(issuerClaims)
	if err != nil {
		return nil, err
	}

	err = VerifyDisclosures(issuerClaims, disclosures)
	if err != nil {
		return nil, err
	}

	merged, err := MergeDisclosedClaims(issuerClaims, disclosures)
	if err != nil {
		return nil, err
	}

	sub, _ := merged["sub"].(string)
	sub = strings.TrimSpace(sub)
	if sub == "" {
		cnfJWK, cnfErr := CNFJWKFromClaims(merged)
		if cnfErr != nil {
			return nil, fmt.Errorf("credential missing sub")
		}
		sub, err = DIDJWKFromPublicJWK(cnfJWK)
		if err != nil {
			return nil, err
		}
		merged["sub"] = sub
	}

	return merged, nil
}

func validateNotBeforeIfPresent(claims jwt.MapClaims) error {
	nbfVal, ok := claims["nbf"]
	if !ok {
		return nil
	}

	nbf, err := numericDate(nbfVal)
	if err != nil {
		return err
	}

	if time.Now().UTC().Before(nbf) {
		return fmt.Errorf("token not yet valid")
	}

	return nil
}

func numericDate(v any) (time.Time, error) {
	switch t := v.(type) {
	case float64:
		return time.Unix(int64(t), 0).UTC(), nil
	case json.Number:
		sec, err := t.Int64()
		if err != nil {
			return time.Time{}, err
		}
		return time.Unix(sec, 0).UTC(), nil
	default:
		return time.Time{}, fmt.Errorf("invalid numeric date")
	}
}
