package sdjwt

import (
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// CNFJWKFromClaims extracts the holder binding key from credential claims.
func CNFJWKFromClaims(claims jwt.MapClaims) (JWK, error) {
	rawCNF, ok := claims["cnf"]

	if !ok {
		return JWK{}, fmt.Errorf("credential missing cnf")
	}

	cnf, ok := rawCNF.(map[string]any)

	if !ok {
		return JWK{}, fmt.Errorf("credential cnf must be an object")
	}

	rawJWK, ok := cnf["jwk"]

	if !ok {
		return JWK{}, fmt.Errorf("credential missing cnf.jwk")
	}

	return JWKFromAny(rawJWK)
}

// VerifyKB validates the holder KB-JWT against cnf.jwk and the presentation context.
func VerifyKB(token, expectedSDHash string, cnfJWK JWK, holderSub, nonce, clientID string) error {
	parsed, err := jwt.NewParser(
		jwt.WithValidMethods([]string{"ES256"}),
	).Parse(token, func(t *jwt.Token) (any, error) {
		return holderVerificationKey(cnfJWK, t)
	})

	if err != nil {
		return fmt.Errorf("kb jwt signature: %w", err)
	}

	err = validateKBHeader(parsed)
	if err != nil {
		return err
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return fmt.Errorf("kb jwt claims are invalid")
	}

	if err := validateKBIssuedAt(claims); err != nil {
		return err
	}

	claimNonce, _ := claims["nonce"].(string)
	if strings.TrimSpace(nonce) == "" || claimNonce != nonce {
		return fmt.Errorf("kb jwt nonce mismatch")
	}

	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return fmt.Errorf("kb jwt verification requires client_id")
	}

	if !audienceMatches(claims["aud"], clientID) {
		return fmt.Errorf("kb jwt aud mismatch")
	}

	hash, _ := claims["sd_hash"].(string)
	if strings.TrimSpace(hash) == "" {
		return fmt.Errorf("kb jwt missing sd_hash")
	}
	if expectedSDHash == "" || hash != expectedSDHash {
		return fmt.Errorf("kb jwt sd_hash mismatch")
	}

	if iss, ok := claims["iss"].(string); ok && strings.TrimSpace(iss) != "" {
		if strings.TrimSpace(iss) != holderSub {
			return fmt.Errorf("kb jwt iss does not match credential holder")
		}
	}

	return nil
}

// validateKBIssuedAt checks that iat is present and within an acceptable window.
// jwt.WithIssuedAt() is intentionally not used because it rejects tokens whose iat
// is even slightly in the future, which breaks wallets with minor clock skew.
func validateKBIssuedAt(claims jwt.MapClaims) error {
	iat, err := claims.GetIssuedAt()
	if err != nil || iat == nil {
		return fmt.Errorf("kb jwt missing iat")
	}
	now := time.Now().UTC()
	if iat.After(now.Add(10 * time.Minute)) {
		return fmt.Errorf("kb jwt iat is too far in the future")
	}
	if iat.Before(now.Add(-5 * time.Minute)) {
		return fmt.Errorf("kb jwt iat is too old")
	}
	return nil
}

func audienceMatches(raw any, expected string) bool {
	switch aud := raw.(type) {
	case string:
		return strings.TrimSpace(aud) == expected
	case []any:
		for _, item := range aud {
			s, ok := item.(string)
			if ok && strings.TrimSpace(s) == expected {
				return true
			}
		}
	}

	return false
}
