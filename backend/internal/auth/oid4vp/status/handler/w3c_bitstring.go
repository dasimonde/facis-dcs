package handler

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"digital-contracting-service/internal/auth/oid4vp/status"
	"digital-contracting-service/internal/auth/oid4vp/status/codec"
	"digital-contracting-service/internal/auth/oid4vp/status/envelope"
	"digital-contracting-service/internal/auth/oid4vp/status/fetch"

	"github.com/golang-jwt/jwt/v5"
)

type W3CBitstring struct {
	Fetcher *fetch.Client
	Trust   *status.TrustConfig
	Now     func() time.Time
}

func (h *W3CBitstring) Mechanism() status.Mechanism {
	return status.MechanismW3CBitstring
}

func (h *W3CBitstring) Check(
	ctx context.Context,
	_ status.VerifiedCredential,
	ref status.Reference,
) (status.Result, error) {
	if err := requireStatusTrust(h.Trust); err != nil {
		return status.Result{}, err
	}

	var response fetch.Response
	if ref.Prefetched != nil {
		response = *ref.Prefetched
	} else {
		var err error
		response, err = status.FetchStatusList(ctx, h.Fetcher, ref.URI, fetch.RequestOpts{})
		if err != nil {
			return status.Result{}, status.ErrStatusRetrieval
		}
	}

	encodedList, purpose, err := h.extractW3CEncodedList(response)
	if err != nil {
		return status.Result{}, err
	}

	if ref.Purpose != "" && purpose != "" && ref.Purpose != purpose {
		return status.Result{}, status.ErrPurposeMismatch
	}

	compressed, err := codec.DecodeMultibaseBase64URL(encodedList)
	if err != nil {
		return status.Result{}, status.ErrStatusDecoding
	}

	bitstring, err := codec.GZIPDecompressLimited(compressed, 0)
	if err != nil {
		return status.Result{}, status.ErrStatusDecompression
	}

	width := ref.StatusSize
	if width == 0 {
		width = 1
	}

	value, err := codec.ReadStatusValue(bitstring, ref.Index, width, codec.MSBFirst)
	if err != nil {
		if errors.Is(err, codec.ErrIndexOutOfRange) {
			return status.Result{}, status.ErrIndexOutOfRange
		}
		return status.Result{}, err
	}

	return status.MapW3CResult(ref, value), nil
}

func (h *W3CBitstring) extractW3CEncodedList(response fetch.Response) (string, string, error) {
	contentType := envelope.NormalizeContentType(response.ContentType)
	body := response.Body

	switch {
	case contentType == "application/vc+jwt" || status.IsLikelyJWT(body):
		claims, err := h.verifyJWT(body)
		if err != nil {
			return "", "", mapStatusVerifyError(err)
		}
		return extractEncodedListFromClaims(claims)
	case contentType == "application/vc+cose":
		claims, err := h.verifyCOSE(body)
		if err != nil {
			return "", "", mapStatusVerifyError(err)
		}
		if normalized, ok := status.NormalizeAnyMap(claims); ok {
			claims = normalized
		}
		return extractEncodedListFromMap(claims)
	case contentType == "application/vc" || contentType == "application/ld+json" || status.LooksLikeJSON(body):
		if status.IsLikelyJWT(body) {
			return "", "", status.ErrUnsupportedMediaType
		}
		claims, err := h.verifySecuredW3CDocument(body)
		if err != nil {
			return "", "", mapStatusVerifyError(err)
		}
		return extractEncodedListFromMap(claims)
	default:
		return "", "", status.ErrUnsupportedMediaType
	}
}

func (h *W3CBitstring) verifyJWT(body []byte) (map[string]any, error) {
	if err := requireStatusTrust(h.Trust); err != nil {
		return nil, err
	}
	verified, err := envelope.VerifyES256JWT(body, func(issuer string, _ *jwt.Token) (*ecdsa.PublicKey, error) {
		return h.Trust.ResolveECDSAPublicKey(issuer)
	})
	if err != nil {
		return nil, err
	}
	return verified.Claims, nil
}

func (h *W3CBitstring) verifyCOSE(body []byte) (map[string]any, error) {
	if err := requireStatusTrust(h.Trust); err != nil {
		return nil, err
	}
	return envelope.VerifyCOSEVC(body, envelope.COSEVerifier{
		ResolveECDSA: h.Trust.ResolveECDSAPublicKey,
	})
}

func (h *W3CBitstring) verifySecuredW3CDocument(body []byte) (map[string]any, error) {
	var document map[string]any
	if err := json.Unmarshal(body, &document); err != nil {
		return nil, err
	}
	if document["proof"] == nil {
		return nil, status.ErrStatusListNotSecured
	}
	if err := requireStatusTrust(h.Trust); err != nil {
		return nil, err
	}

	proof, err := extractW3CProof(document)
	if err != nil {
		return nil, err
	}
	proofType, _ := proof["type"].(string)
	switch strings.TrimSpace(proofType) {
	case envelope.ProofTypeEd25519Signature2020:
		return envelope.VerifyEd25519Signature2020Credential(body, envelope.Ed25519Signature2020Verifier{
			ResolveEd25519: h.Trust.ResolveEd25519PublicKey,
		})
	default:
		return h.verifyDataIntegrity(body)
	}
}

func extractW3CProof(document map[string]any) (map[string]any, error) {
	raw := document["proof"]
	switch proof := raw.(type) {
	case map[string]any:
		return proof, nil
	case []any:
		if len(proof) == 0 {
			return nil, fmt.Errorf("credential proof is empty")
		}
		first, ok := proof[0].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("credential proof has invalid shape")
		}
		return first, nil
	default:
		return nil, fmt.Errorf("credential is missing proof")
	}
}

func (h *W3CBitstring) verifyDataIntegrity(body []byte) (map[string]any, error) {
	var document map[string]any
	if err := json.Unmarshal(body, &document); err != nil {
		return nil, err
	}
	if document["proof"] == nil {
		return nil, status.ErrStatusListNotSecured
	}
	if err := requireStatusTrust(h.Trust); err != nil {
		return nil, err
	}
	document, err := envelope.VerifyDataIntegrityCredential(body, envelope.DataIntegrityVerifier{
		ResolveECDSA:   h.Trust.ResolveECDSAPublicKey,
		ResolveEd25519: h.Trust.ResolveEd25519PublicKey,
	})
	if err != nil {
		return nil, err
	}
	return document, nil
}

func mapStatusVerifyError(err error) error {
	if errors.Is(err, status.ErrStatusTrustNotConfigured) ||
		errors.Is(err, status.ErrStatusListNotSecured) {
		return err
	}
	return status.ErrStatusSignature
}

func extractEncodedListFromClaims(claims map[string]any) (string, string, error) {
	types, _ := claims["type"].([]any)
	if !hasCredentialType(types, "BitstringStatusListCredential") {
		return "", "", status.ErrWrongStatusListType
	}
	return extractEncodedListFromMap(claims)
}

func extractEncodedListFromMap(claims map[string]any) (string, string, error) {
	subject, ok := claims["credentialSubject"].(map[string]any)
	if !ok {
		return "", "", status.ErrWrongStatusListType
	}
	subjectType := subjectTypeValue(subject["type"])
	if !subjectTypeMatches(subjectType, "BitstringStatusList") {
		return "", "", status.ErrWrongStatusListType
	}

	encodedList, _ := subject["encodedList"].(string)
	if strings.TrimSpace(encodedList) == "" {
		return "", "", status.ErrStatusDecoding
	}
	purpose, _ := subject["statusPurpose"].(string)
	return encodedList, purpose, nil
}

func hasCredentialType(types []any, want string) bool {
	for _, item := range types {
		if s, ok := item.(string); ok && s == want {
			return true
		}
	}
	return false
}

func subjectTypeValue(raw any) any {
	return raw
}

func subjectTypeMatches(raw any, want string) bool {
	switch value := raw.(type) {
	case string:
		return value == want
	case []any:
		return hasCredentialType(value, want)
	default:
		return false
	}
}
