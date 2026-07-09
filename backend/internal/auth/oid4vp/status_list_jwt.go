package oid4vp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"digital-contracting-service/internal/auth/oid4vp/sdjwt"

	"github.com/golang-jwt/jwt/v5"
)

const jwtStatusListTyp = "statuslist+jwt"

// StatusListJWTPayloadVerifier returns verified status-list JWT payload bytes.
type StatusListJWTPayloadVerifier func(ctx context.Context, token string) ([]byte, error)

var (
	statusListJWTTrust           *TrustConfig
	statusListJWTPayloadVerifier StatusListJWTPayloadVerifier = verifyStatusListJWTPayload
)

// ConfigureStatusListJWTVerification wires trust-backed JWS verification.
func ConfigureStatusListJWTVerification(trust *TrustConfig, skipJWSVerify bool) {
	statusListJWTTrust = trust
	if skipJWSVerify {
		statusListJWTPayloadVerifier = parseUnverifiedJWTPayload
		return
	}
	statusListJWTPayloadVerifier = verifyStatusListJWTPayload
}

// verifyJWTStatusList fetches a Status List Token (JWT, typ statuslist+jwt) and checks idx.
func verifyJWTStatusList(ctx context.Context, uri string, index uint32) error {
	body, err := fetchStatusListBody(ctx, uri)
	if err != nil {
		return err
	}

	return verifyJWTStatusListBodyWithContext(ctx, body, index)
}

func verifyJWTStatusListBody(body []byte, index uint32) error {
	return verifyJWTStatusListBodyWithContext(context.Background(), body, index)
}

func verifyJWTStatusListBodyWithContext(ctx context.Context, body []byte, index uint32) error {
	tokenStr := strings.TrimSpace(string(body))

	if tokenStr == "" {
		return fmt.Errorf("empty JWT status list response")
	}

	if err := validateStatusListJWTHeader(tokenStr); err != nil {
		return err
	}

	payload, err := statusListJWTPayloadVerifier(ctx, tokenStr)

	if err != nil {
		return fmt.Errorf("verify JWT status list token: %w", err)
	}

	dec := json.NewDecoder(strings.NewReader(string(payload)))
	dec.UseNumber()
	var claims map[string]any

	if err := dec.Decode(&claims); err != nil {
		return fmt.Errorf("parse JWT status list claims: %w", err)
	}

	if err := validateJWTTemporalClaims(claims, time.Now()); err != nil {
		return err
	}

	statusList, ok := claims["status_list"].(map[string]any)
	if !ok {
		return fmt.Errorf("JWT status list token missing status_list claim")
	}

	encoded, bitsPerEntry, err := parseJWTStatusListEncodedList(statusList)
	if err != nil {
		return err
	}

	status, err := queryTokenStatusFromEncodedList(encoded, index, bitsPerEntry)
	if err != nil {
		return fmt.Errorf("query JWT status list: %w", err)
	}

	return ensureActiveStatus(status, index)
}

// parseJWTStatusListEncodedList extracts the compressed bitstring from a JWT status_list claim.
// Standard IETF tokens use integer bits + lst string; some issuers put the
// encoded bitstring directly in bits as a string.
func parseJWTStatusListEncodedList(statusList map[string]any) (encoded string, bitsPerEntry uint32, err error) {
	if lst, ok := statusList["lst"].(string); ok {
		lst = strings.TrimSpace(lst)
		if lst != "" {
			bitsPerEntry, err = parseTokenStatusListBits(statusList["bits"])
			if err != nil {
				return "", 0, err
			}
			return lst, bitsPerEntry, nil
		}
	}

	if bitsStr, ok := statusList["bits"].(string); ok {
		bitsStr = strings.TrimSpace(bitsStr)
		if bitsStr != "" {
			return bitsStr, 1, nil
		}
	}

	return "", 0, fmt.Errorf("JWT status list token missing status_list.lst")
}

func validateStatusListJWTHeader(token string) error {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return fmt.Errorf("JWT status list response is not a compact JWT")
	}

	headerRaw, err := decodeBase64URLOrStd(parts[0])
	if err != nil {
		return fmt.Errorf("decode JWT status list header: %w", err)
	}

	var header struct {
		Typ string `json:"typ"`
		Alg string `json:"alg"`
	}

	if err := json.Unmarshal(headerRaw, &header); err != nil {
		return fmt.Errorf("parse JWT status list header: %w", err)
	}

	if typ := strings.TrimSpace(header.Typ); typ != "" && !strings.EqualFold(typ, jwtStatusListTyp) && !strings.EqualFold(typ, "application/"+jwtStatusListTyp) {
		return fmt.Errorf("JWT status list typ must be %q, got %q", jwtStatusListTyp, typ)
	}

	return nil
}

func verifyStatusListJWTPayload(_ context.Context, token string) ([]byte, error) {
	if statusListJWTTrust == nil {
		return nil, fmt.Errorf("status list JWT trust config is not loaded")
	}

	parsed, err := jwt.NewParser(jwt.WithValidMethods([]string{"ES256"})).Parse(token, func(t *jwt.Token) (any, error) {
		return sdjwt.ResolveIssuerVerificationKey(statusListJWTTrust, t)
	})
	if err != nil {
		return nil, fmt.Errorf("verify status list JWT signature: %w", err)
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("status list JWT claims are invalid")
	}

	payload, err := json.Marshal(claims)
	if err != nil {
		return nil, fmt.Errorf("marshal status list JWT claims: %w", err)
	}

	return payload, nil
}

func parseUnverifiedJWTPayload(_ context.Context, token string) ([]byte, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("not a compact JWT")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		payload, err = base64.URLEncoding.DecodeString(parts[1])
	}

	if err != nil {
		return nil, fmt.Errorf("decode JWT status list payload: %w", err)
	}

	return payload, nil
}

func parseTokenStatusListBits(raw any) (uint32, error) {
	bits, ok := parseUint32(raw)
	if !ok {
		return 0, fmt.Errorf("JWT status list token missing or invalid status_list.bits")
	}

	switch bits {
	case 1, 2, 4, 8:
		return bits, nil
	default:
		return 0, fmt.Errorf("JWT status list bits must be one of 1, 2, 4, 8; got %d", bits)
	}
}

func validateJWTTemporalClaims(claims map[string]any, now time.Time) error {
	if exp, ok, err := numericDate(claims["exp"]); err != nil {
		return fmt.Errorf("JWT status list exp is invalid: %w", err)
	} else if ok && !now.Before(exp) {
		return fmt.Errorf("JWT status list token is expired")
	}

	if nbf, ok, err := numericDate(claims["nbf"]); err != nil {
		return fmt.Errorf("JWT status list nbf is invalid: %w", err)
	} else if ok && now.Before(nbf) {
		return fmt.Errorf("JWT status list token is not valid yet")
	}

	return nil
}

func numericDate(raw any) (time.Time, bool, error) {
	if raw == nil {
		return time.Time{}, false, nil
	}

	var seconds int64

	switch v := raw.(type) {
	case json.Number:
		n, err := strconv.ParseInt(v.String(), 10, 64)
		if err != nil {
			return time.Time{}, false, err
		}
		seconds = n
	case float64:
		if v != float64(int64(v)) {
			return time.Time{}, false, fmt.Errorf("must be an integer NumericDate")
		}
		seconds = int64(v)
	case int64:
		seconds = v
	case int:
		seconds = int64(v)
	default:
		return time.Time{}, false, fmt.Errorf("unsupported NumericDate type %T", raw)
	}

	return time.Unix(seconds, 0), true, nil
}
