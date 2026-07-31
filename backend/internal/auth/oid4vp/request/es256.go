package request

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

const oauthAuthzReqJWTType = "oauth-authz-req+jwt"

func signES256JWT(kid string, claims jwt.MapClaims, extraHeaders map[string]any, sign func(signingInput string) ([]byte, error)) (string, error) {
	kid = strings.TrimSpace(kid)

	if kid == "" {
		return "", fmt.Errorf("signing kid is required")
	}

	header := map[string]any{
		"alg": "ES256",
		"typ": oauthAuthzReqJWTType,
		"kid": kid,
	}
	for k, v := range extraHeaders {
		header[k] = v
	}

	headerJSON, err := json.Marshal(header)

	if err != nil {
		return "", fmt.Errorf("marshal jwt header: %w", err)
	}

	payloadJSON, err := json.Marshal(claims)

	if err != nil {
		return "", fmt.Errorf("marshal jwt claims: %w", err)
	}

	signingInput := base64.RawURLEncoding.EncodeToString(headerJSON) + "." + base64.RawURLEncoding.EncodeToString(payloadJSON)
	sig, err := sign(signingInput)

	if err != nil {
		return "", err
	}

	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}
