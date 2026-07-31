package request

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type captureSigner struct {
	claims jwt.MapClaims
}

func (s *captureSigner) SignAuthorizationRequestJWT(claims jwt.MapClaims) (string, error) {
	s.claims = claims
	return "signed", nil
}

func TestBuildJWTIncludesWalletNonceAndClientMetadata(t *testing.T) {
	signer := &captureSigner{}
	_, err := BuildJWT(signer, Params{
		ClientID:    "dcs-client",
		ResponseURI: "https://rp.example/cb",
		State:       "state-1",
		Nonce:       "nonce-1",
		WalletNonce: "wallet-1",
		ExpiresAt:   time.Now().UTC().Add(time.Minute),
		DCQLQuery: map[string]any{
			"credentials": []any{map[string]any{"id": "q1"}},
		},
	})
	if err != nil {
		t.Fatalf("BuildJWT returned error: %v", err)
	}

	if got := signer.claims["wallet_nonce"]; got != "wallet-1" {
		t.Fatalf("wallet_nonce mismatch: got %v", got)
	}

	clientMetadata, ok := signer.claims["client_metadata"].(map[string]any)
	if !ok {
		t.Fatalf("client_metadata missing or wrong type: %T", signer.claims["client_metadata"])
	}
	vpFormats, ok := clientMetadata["vp_formats_supported"].(map[string]any)
	if !ok {
		t.Fatalf("vp_formats_supported missing or wrong type")
	}
	format, ok := vpFormats["dc+sd-jwt"].(map[string]any)
	if !ok {
		t.Fatalf("dc+sd-jwt format metadata missing")
	}
	if _, ok := format["sd-jwt_alg_values"]; !ok {
		t.Fatalf("sd-jwt_alg_values missing")
	}
	if _, ok := format["kb-jwt_alg_values"]; !ok {
		t.Fatalf("kb-jwt_alg_values missing")
	}
}

func TestSignES256JWTIncludesJWKHeader(t *testing.T) {
	token, err := signES256JWT("kid-1", jwt.MapClaims{"a": "b"}, map[string]any{
		"jwk": map[string]string{
			"kty": "EC",
			"crv": "P-256",
			"x":   "x",
			"y":   "y",
		},
	}, func(signingInput string) ([]byte, error) {
		return []byte{1, 2, 3}, nil
	})
	if err != nil {
		t.Fatalf("signES256JWT returned error: %v", err)
	}

	parts := splitJWT(token)
	rawHeader, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("decode header: %v", err)
	}

	var header map[string]any
	if err := json.Unmarshal(rawHeader, &header); err != nil {
		t.Fatalf("unmarshal header: %v", err)
	}

	if header["typ"] != oauthAuthzReqJWTType {
		t.Fatalf("typ mismatch: %v", header["typ"])
	}
	if _, ok := header["jwk"]; !ok {
		t.Fatalf("jwk header missing")
	}
}

func TestSignES256JWTIncludesX5CHeader(t *testing.T) {
	token, err := signES256JWT("kid-1", jwt.MapClaims{"a": "b"}, map[string]any{
		"x5c": []string{"deadbeef"},
	}, func(signingInput string) ([]byte, error) {
		return []byte{1, 2, 3}, nil
	})
	if err != nil {
		t.Fatalf("signES256JWT returned error: %v", err)
	}

	parts := splitJWT(token)
	rawHeader, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("decode header: %v", err)
	}

	var header map[string]any
	if err := json.Unmarshal(rawHeader, &header); err != nil {
		t.Fatalf("unmarshal header: %v", err)
	}

	if _, ok := header["jwk"]; ok {
		t.Fatalf("jwk header should not be present when signing with x5c")
	}
	x5c, ok := header["x5c"].([]any)
	if !ok || len(x5c) != 1 || x5c[0] != "deadbeef" {
		t.Fatalf("x5c header missing or malformed: %v", header["x5c"])
	}
}

func splitJWT(token string) []string {
	parts := make([]string, 0, 3)
	start := 0
	for i := 0; i < len(token); i++ {
		if token[i] == '.' {
			parts = append(parts, token[start:i])
			start = i + 1
		}
	}
	parts = append(parts, token[start:])
	return parts
}
