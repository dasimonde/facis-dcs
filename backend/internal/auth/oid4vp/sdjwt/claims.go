package sdjwt

import (
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// RolesFromClaims returns the roles array from merged credential claims.
func RolesFromClaims(claims jwt.MapClaims) ([]string, error) {
	raw, ok := claims["roles"]

	if !ok {
		return nil, fmt.Errorf("credential missing roles")
	}

	arr, ok := raw.([]any)

	if !ok {
		return nil, fmt.Errorf("credential roles must be an array")
	}

	out := make([]string, 0, len(arr))

	for _, item := range arr {
		s, ok := item.(string)
		if !ok || strings.TrimSpace(s) == "" {
			return nil, fmt.Errorf("credential roles must be strings")
		}
		out = append(out, s)
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("credential roles are empty")
	}

	return out, nil
}

// OrganizationFromClaims returns the organization from merged credential claims.
func OrganizationFromClaims(claims jwt.MapClaims) (string, error) {
	organization, _ := claims["organization"].(string)
	organization = strings.TrimSpace(organization)

	if organization == "" {
		return "", fmt.Errorf("credential missing organization")
	}

	return organization, nil
}
