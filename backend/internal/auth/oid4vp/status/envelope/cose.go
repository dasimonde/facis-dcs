package envelope

import (
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/fxamacker/cbor/v2"
)

type COSEVerifier struct {
	ResolveECDSA func(issuer string) (*ecdsa.PublicKey, error)
}

func SignCOSEVC(document map[string]any, privateKey *ecdsa.PrivateKey, contentType string) ([]byte, error) {
	if contentType == "" {
		contentType = "application/vc+cose"
	}
	payload, err := cbor.Marshal(document)
	if err != nil {
		return nil, err
	}
	protected, err := cbor.Marshal(map[int64]any{
		coseHeaderAlgorithm:   coseAlgES256,
		coseHeaderContentType: contentType,
	})
	if err != nil {
		return nil, err
	}
	return signCOSE(protected, payload, privateKey)
}

func VerifyCOSEVC(raw []byte, verifier COSEVerifier) (map[string]any, error) {
	if verifier.ResolveECDSA == nil {
		return nil, fmt.Errorf("ecdsa resolver is required")
	}
	payload, err := verifyCOSE(raw, verifier.ResolveECDSA)
	if err != nil {
		return nil, err
	}
	var document map[string]any
	if err := json.Unmarshal(payload, &document); err == nil {
		return document, nil
	}
	if err := cbor.Unmarshal(payload, &document); err != nil {
		return nil, err
	}
	return document, nil
}

func IssuerFromClaims(claims map[string]any) string {
	if iss, ok := claims["iss"].(string); ok {
		return strings.TrimSpace(iss)
	}
	if issuer, ok := claims["issuer"].(string); ok {
		return strings.TrimSpace(issuer)
	}
	return ""
}

func SubjectFromClaims(claims map[string]any) string {
	if sub, ok := claims["sub"].(string); ok {
		return strings.TrimSpace(sub)
	}
	return ""
}

func ExpFromClaims(claims map[string]any) (int64, bool) {
	switch v := claims["exp"].(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case uint64:
		return int64(v), true
	case float64:
		return int64(v), true
	default:
		return 0, false
	}
}

func IatFromClaims(claims map[string]any) (int64, bool) {
	switch v := claims["iat"].(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case uint64:
		return int64(v), true
	case float64:
		return int64(v), true
	default:
		return 0, false
	}
}

func NbfFromClaims(claims map[string]any) (int64, bool) {
	switch v := claims["nbf"].(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case uint64:
		return int64(v), true
	case float64:
		return int64(v), true
	default:
		return 0, false
	}
}
