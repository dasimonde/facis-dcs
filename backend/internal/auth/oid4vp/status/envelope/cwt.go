package envelope

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"fmt"
	"math/big"
	"strings"

	"github.com/fxamacker/cbor/v2"
)

// IETF Status List CWT profile supported by this package:
//   - COSE_Sign1 (tag 18) with ES256
//   - protected header key 16 (type) as textual "application/statuslist+cwt"
//   - COSE kid (header label 4) as byte string containing the UTF-8 JWK kid value
//
// COSE_Mac0, numeric CoAP Content-Format type IDs, and other algorithms are not supported.
//
// Claim labels follow RFC 8392 and draft-ietf-oauth-status-list-21.
const (
	cwtProtectedHeaderType = int64(16)
	cwtMediaType           = "application/statuslist+cwt"

	cwtClaimIss        = int64(1)
	cwtClaimSub        = int64(2)
	cwtClaimExp        = int64(4)
	cwtClaimNbf        = int64(5)
	cwtClaimIat        = int64(6)
	cwtClaimStatusList = int64(65533)
	cwtClaimTTL        = int64(65534)
)

type CWTVerifier struct {
	// StatusListURI scopes COSE kid lookup when multiple trusted keys share the same kid.
	StatusListURI string
	// ResolveECDSAByKID resolves the verification key from COSE header kid (RFC 9052 key 4).
	ResolveECDSAByKID func(statusListURI, kid string) (*ecdsa.PublicKey, error)
	// ResolveECDSA is an optional fallback when claim 1 (iss) is present.
	ResolveECDSA func(issuer string) (*ecdsa.PublicKey, error)
}

// SignStatusListCWT builds a draft-21 Status List CWT (COSE_Sign1, integer claim labels).
// claims uses JWT-style string keys (sub, iat, exp, status_list{bits,lst}); iss is optional.
// kid, when non-empty, is written to the COSE unprotected header for key lookup.
func SignStatusListCWT(claims map[string]any, privateKey *ecdsa.PrivateKey, kid string) ([]byte, error) {
	payload, err := encodeCWTClaimsSet(claims)
	if err != nil {
		return nil, err
	}
	protected, err := cbor.Marshal(map[int64]any{
		coseHeaderAlgorithm:    coseAlgES256,
		cwtProtectedHeaderType: cwtMediaType,
	})
	if err != nil {
		return nil, err
	}
	unprotected := map[int64]any{}
	if kid := strings.TrimSpace(kid); kid != "" {
		unprotected[coseHeaderKID] = []byte(kid)
	}
	return signCOSESign1(protected, unprotected, payload, privateKey)
}

// VerifyStatusListCWT verifies a Status List CWT and returns normalized string-key claims.
func VerifyStatusListCWT(raw []byte, verifier CWTVerifier) (map[string]any, error) {
	if verifier.ResolveECDSAByKID == nil && verifier.ResolveECDSA == nil {
		return nil, fmt.Errorf("ecdsa resolver is required")
	}

	protected, unprotected, payload, signature, err := unmarshalCOSESign1Full(raw)
	if err != nil {
		return nil, err
	}
	if err := validateStatusListCWTProtected(protected); err != nil {
		return nil, err
	}

	claims, err := decodeCWTClaimsSet(payload)
	if err != nil {
		return nil, err
	}
	if err := validateStatusListCWTClaims(claims); err != nil {
		return nil, err
	}

	pub, err := resolveCWTVerificationKey(protected, unprotected, claims, verifier)
	if err != nil {
		return nil, err
	}

	sigStructure, err := cbor.Marshal([]any{coseSign1Context, protected, []byte{}, payload})
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(sigStructure)
	if len(signature) != 64 {
		return nil, fmt.Errorf("invalid COSE signature length %d", len(signature))
	}
	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:])
	if !ecdsa.Verify(pub, digest[:], r, s) {
		return nil, fmt.Errorf("cwt signature verification failed")
	}

	return normalizeCWTClaims(claims)
}

func resolveCWTVerificationKey(
	protected []byte,
	unprotected map[int64]any,
	claims map[int64]any,
	verifier CWTVerifier,
) (*ecdsa.PublicKey, error) {
	kid, err := coseKIDFromHeaders(protected, unprotected)
	if err != nil {
		return nil, err
	}
	if kid != "" {
		if verifier.ResolveECDSAByKID == nil {
			return nil, fmt.Errorf("cwt COSE kid %q present but kid resolver is not configured", kid)
		}
		return verifier.ResolveECDSAByKID(verifier.StatusListURI, kid)
	}

	issuer, err := cwtClaimString(claims[cwtClaimIss])
	if err == nil && verifier.ResolveECDSA != nil {
		return verifier.ResolveECDSA(issuer)
	}
	if err == nil {
		return nil, fmt.Errorf("cwt claim 1 (iss) present but issuer resolver is not configured")
	}
	return nil, fmt.Errorf("cwt verification requires COSE kid or claim 1 (iss)")
}

func coseKIDFromHeaders(protected []byte, unprotected map[int64]any) (string, error) {
	var protectedMap map[int64]any
	if err := cbor.Unmarshal(protected, &protectedMap); err != nil {
		return "", fmt.Errorf("invalid cwt protected header: %w", err)
	}

	_, protectedPresent := protectedMap[coseHeaderKID]
	_, unprotectedPresent := unprotected[coseHeaderKID]
	if protectedPresent && unprotectedPresent {
		return "", fmt.Errorf("cose kid must not appear in both protected and unprotected headers")
	}
	if protectedPresent {
		return coseHeaderKIDValue(protectedMap[coseHeaderKID])
	}
	if unprotectedPresent {
		return coseHeaderKIDValue(unprotected[coseHeaderKID])
	}
	return "", nil
}

func coseHeaderKIDValue(raw any) (string, error) {
	b, ok := raw.([]byte)
	if !ok {
		return "", fmt.Errorf("cose kid must be a byte string")
	}
	kid := strings.TrimSpace(string(b))
	if kid == "" {
		return "", fmt.Errorf("cose kid must not be empty")
	}
	return kid, nil
}

func validateStatusListCWTProtected(protected []byte) error {
	var hdr map[int64]any
	if err := cbor.Unmarshal(protected, &hdr); err != nil {
		return fmt.Errorf("invalid cwt protected header: %w", err)
	}
	alg, ok := hdr[coseHeaderAlgorithm].(int64)
	if !ok || alg != coseAlgES256 {
		return fmt.Errorf("unsupported cwt COSE algorithm %v", hdr[coseHeaderAlgorithm])
	}
	typ, err := cwtClaimString(hdr[cwtProtectedHeaderType])
	if err != nil || typ != cwtMediaType {
		return fmt.Errorf("cwt protected header type must be %q", cwtMediaType)
	}
	return nil
}

func validateStatusListCWTClaims(claims map[int64]any) error {
	if _, err := cwtClaimString(claims[cwtClaimSub]); err != nil {
		return fmt.Errorf("cwt claim 2 (sub): %w", err)
	}
	if _, ok := cwtClaimUint(claims[cwtClaimIat]); !ok {
		return fmt.Errorf("cwt claim 6 (iat) is required")
	}
	if raw, exists := claims[cwtClaimIss]; exists {
		if _, err := cwtClaimString(raw); err != nil {
			return fmt.Errorf("cwt claim 1 (iss): %w", err)
		}
	}
	if err := validateOptionalCWTUintClaim(claims, cwtClaimExp, "exp", false); err != nil {
		return err
	}
	if err := validateOptionalCWTUintClaim(claims, cwtClaimNbf, "nbf", false); err != nil {
		return err
	}
	if err := validateOptionalCWTUintClaim(claims, cwtClaimTTL, "ttl", true); err != nil {
		return err
	}
	if _, err := decodeCWTStatusList(claims[cwtClaimStatusList]); err != nil {
		return fmt.Errorf("cwt claim 65533 (status_list): %w", err)
	}
	return nil
}

func validateOptionalCWTUintClaim(claims map[int64]any, key int64, name string, positive bool) error {
	raw, ok := claims[key]
	if !ok {
		return nil
	}
	v, ok := cwtClaimUint(raw)
	if !ok {
		return fmt.Errorf("cwt claim %d (%s) must be a non-negative integer", key, name)
	}
	if positive && v == 0 {
		return fmt.Errorf("cwt claim %d (%s) must be positive", key, name)
	}
	return nil
}

func encodeCWTClaimsSet(claims map[string]any) ([]byte, error) {
	out := map[int64]any{}

	if v := strings.TrimSpace(IssuerFromClaims(claims)); v != "" {
		out[cwtClaimIss] = v
	}
	if v := SubjectFromClaims(claims); v != "" {
		out[cwtClaimSub] = v
	}
	if v, ok := IatFromClaims(claims); ok {
		out[cwtClaimIat] = v
	}
	if raw, ok := claims["exp"]; ok {
		v, ok := cwtClaimUint(raw)
		if !ok {
			return nil, fmt.Errorf("exp must be a non-negative integer")
		}
		out[cwtClaimExp] = v
	}
	if raw, ok := claims["nbf"]; ok {
		v, ok := cwtClaimUint(raw)
		if !ok {
			return nil, fmt.Errorf("nbf must be a non-negative integer")
		}
		out[cwtClaimNbf] = v
	}
	if raw, ok := claims["ttl"]; ok {
		v, ok := cwtClaimUint(raw)
		if !ok || v == 0 {
			return nil, fmt.Errorf("ttl must be a positive integer")
		}
		out[cwtClaimTTL] = v
	}

	statusList, ok := claims["status_list"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("status_list claim is required")
	}
	encoded, err := encodeCWTStatusList(statusList)
	if err != nil {
		return nil, err
	}
	out[cwtClaimStatusList] = encoded

	return cbor.Marshal(out)
}

func encodeCWTStatusList(sl map[string]any) (map[string]any, error) {
	bits, ok := cwtClaimUint(sl["bits"])
	if !ok || (bits != 1 && bits != 2 && bits != 4 && bits != 8) {
		return nil, fmt.Errorf("status_list.bits must be 1, 2, 4, or 8")
	}

	lst, ok := sl["lst"].([]byte)
	if !ok || len(lst) == 0 {
		return nil, fmt.Errorf("status_list.lst must be a non-empty byte string")
	}
	return map[string]any{"bits": bits, "lst": lst}, nil
}

func decodeCWTClaimsSet(payload []byte) (map[int64]any, error) {
	var claims map[int64]any
	if err := cbor.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("invalid cwt claims set: %w", err)
	}
	return claims, nil
}

func normalizeCWTClaims(raw map[int64]any) (map[string]any, error) {
	out := map[string]any{}

	if v, err := cwtClaimString(raw[cwtClaimIss]); err == nil {
		out["iss"] = v
	}
	if v, err := cwtClaimString(raw[cwtClaimSub]); err == nil {
		out["sub"] = v
	}
	if v, ok := cwtClaimUint(raw[cwtClaimIat]); ok {
		out["iat"] = int64(v)
	}
	if v, ok := cwtClaimUint(raw[cwtClaimExp]); ok {
		out["exp"] = int64(v)
	}
	if v, ok := cwtClaimUint(raw[cwtClaimNbf]); ok {
		out["nbf"] = int64(v)
	}
	if v, ok := cwtClaimUint(raw[cwtClaimTTL]); ok {
		out["ttl"] = int64(v)
	}

	statusList, err := decodeCWTStatusList(raw[cwtClaimStatusList])
	if err != nil {
		return nil, err
	}
	out["status_list"] = statusList
	return out, nil
}

func decodeCWTStatusList(raw any) (map[string]any, error) {
	m, ok := raw.(map[string]any)
	if !ok {
		if m2, ok := raw.(map[any]any); ok {
			m = make(map[string]any, len(m2))
			for k, v := range m2 {
				key, ok := k.(string)
				if !ok {
					return nil, fmt.Errorf("status_list has non-text key")
				}
				m[key] = v
			}
		} else {
			return nil, fmt.Errorf("status_list must be a map")
		}
	}

	bits, ok := cwtClaimUint(m["bits"])
	if !ok || (bits != 1 && bits != 2 && bits != 4 && bits != 8) {
		return nil, fmt.Errorf("status_list.bits must be 1, 2, 4, or 8")
	}

	lst, err := cwtByteString(m["lst"])
	if err != nil {
		return nil, fmt.Errorf("status_list.lst: %w", err)
	}
	return map[string]any{
		"bits": int64(bits),
		"lst":  lst,
	}, nil
}

func DecodeCWTListBytes(lst any) ([]byte, error) {
	return cwtByteString(lst)
}

func cwtClaimString(raw any) (string, error) {
	s, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("expected text string claim, got %T", raw)
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("empty string claim")
	}
	return s, nil
}

func cwtByteString(raw any) ([]byte, error) {
	v, ok := raw.([]byte)
	if !ok {
		return nil, fmt.Errorf("expected byte string, got %T", raw)
	}
	if len(v) == 0 {
		return nil, fmt.Errorf("empty byte string")
	}
	return v, nil
}

func cwtClaimUint(raw any) (uint64, bool) {
	switch v := raw.(type) {
	case uint64:
		return v, true
	case uint:
		return uint64(v), true
	case uint32:
		return uint64(v), true
	case int64:
		if v < 0 {
			return 0, false
		}
		return uint64(v), true
	case int:
		if v < 0 {
			return 0, false
		}
		return uint64(v), true
	case float64:
		if v < 0 || v != float64(uint64(v)) {
			return 0, false
		}
		return uint64(v), true
	default:
		return 0, false
	}
}
