package request

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Signer signs OpenID4VP authorization request JWTs.
type Signer interface {
	SignAuthorizationRequestJWT(claims jwt.MapClaims) (string, error)
}

// Params are the OpenID4VP parameters encoded in the signed request JWT.
type Params struct {
	ClientID    string
	ResponseURI string
	State       string
	Nonce       string
	WalletNonce string
	ExpiresAt   time.Time
	DCQLQuery   any
}

// BuildJWT creates a signed OpenID4VP authorization request object (JAR).
func BuildJWT(signer Signer, params Params) (string, error) {
	if signer == nil {
		return "", fmt.Errorf("request signer is not configured")
	}

	clientID := strings.TrimSpace(params.ClientID)
	if clientID == "" {
		return "", fmt.Errorf("client_id is required")
	}

	responseURI := strings.TrimSpace(params.ResponseURI)
	if responseURI == "" {
		return "", fmt.Errorf("response_uri is required")
	}

	state := strings.TrimSpace(params.State)
	if state == "" {
		return "", fmt.Errorf("state is required")
	}

	nonce := strings.TrimSpace(params.Nonce)
	if nonce == "" {
		return "", fmt.Errorf("nonce is required")
	}

	if params.DCQLQuery == nil {
		return "", fmt.Errorf("dcql_query is required")
	}

	now := time.Now().UTC()
	exp := params.ExpiresAt.UTC()
	if !exp.After(now) {
		return "", fmt.Errorf("authorization request expiry must be in the future")
	}

	dcqlJSON, err := json.Marshal(params.DCQLQuery)
	if err != nil {
		return "", fmt.Errorf("marshal dcql_query: %w", err)
	}
	var dcql any
	err = json.Unmarshal(dcqlJSON, &dcql)
	if err != nil {
		return "", fmt.Errorf("decode dcql_query: %w", err)
	}

	claims := jwt.MapClaims{
		"iss":           clientID,
		"client_id":     clientID,
		"response_type": "vp_token",
		"response_mode": "direct_post",
		"response_uri":  responseURI,
		"state":         state,
		"nonce":         nonce,
		"dcql_query":    dcql,
		"client_metadata": map[string]any{
			// Both SD-JWT VC format identifiers are advertised: a wallet holding
			// credentials stamped with the pre-rename one — still what most
			// deployed issuers emit — otherwise reads this verifier as unable to
			// consume anything it has.
			"vp_formats_supported": map[string]any{
				"dc+sd-jwt": map[string]any{
					"sd-jwt_alg_values": []string{"ES256"},
					"kb-jwt_alg_values": []string{"ES256"},
				},
				"vc+sd-jwt": map[string]any{
					"sd-jwt_alg_values": []string{"ES256"},
					"kb-jwt_alg_values": []string{"ES256"},
				},
			},
		},
		"iat": now.Unix(),
		"exp": exp.Unix(),
	}

	if walletNonce := strings.TrimSpace(params.WalletNonce); walletNonce != "" {
		claims["wallet_nonce"] = walletNonce
	}

	return signer.SignAuthorizationRequestJWT(claims)
}
