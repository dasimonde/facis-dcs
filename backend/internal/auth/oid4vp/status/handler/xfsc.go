package handler

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"digital-contracting-service/internal/auth/oid4vp/sdjwt"
	"digital-contracting-service/internal/auth/oid4vp/status"
	"digital-contracting-service/internal/auth/oid4vp/status/codec"
	"digital-contracting-service/internal/auth/oid4vp/status/envelope"
	"digital-contracting-service/internal/auth/oid4vp/status/fetch"

	"github.com/golang-jwt/jwt/v5"
)

// XFSC verifies status lists from eclipse-xfsc/statuslist-service.
//
// A status list decides whether a credential is revoked, so it is only believed
// when it is signed and that signature verifies. An unsigned list states a
// revocation status nobody vouched for, and anyone who can answer the URL can
// write it — so there is no configuration under which one is accepted.
type XFSC struct {
	Fetcher *fetch.Client
	Trust   *status.TrustConfig
}

func (h *XFSC) Mechanism() status.Mechanism {
	return status.MechanismXFSC
}

func (h *XFSC) Check(
	ctx context.Context,
	credential status.VerifiedCredential,
	ref status.Reference,
) (status.Result, error) {
	client := h.Fetcher
	if client == nil {
		client = fetch.NewClient()
	}

	signedResp, err := status.FetchStatusList(ctx, client, ref.URI, fetch.RequestOpts{
		ContentType: status.XFSCSignedContentType,
	})
	if err != nil {
		return status.Result{}, status.ErrStatusRetrieval
	}
	verified, err := h.verifyStatusListJWT(UnwrapStatusListJWTBody(signedResp.Body))
	if err != nil {
		return status.Result{}, status.ErrStatusSignature
	}
	return h.resultFromSignedClaims(credential, ref, verified.Claims)
}

func (h *XFSC) resultFromSignedClaims(credential status.VerifiedCredential, ref status.Reference, claims map[string]any) (status.Result, error) {
	// Whose revocation statement this is (ADR-34): the signature says a trusted
	// issuer published this list, not that it is the issuer of the credential
	// whose status it carries.
	listIssuer, _ := claims["iss"].(string)
	if err := status.RequireCredentialIssuer(credential, listIssuer); err != nil {
		return status.Result{}, err
	}

	subject, _ := claims["sub"].(string)
	if !status.SubjectMatchesURI(subject, ref.URI) {
		log.Printf(
			"xfsc status list: jwt sub %q does not match credential uri %q (known XFSC statuslist-service issue)",
			strings.TrimSpace(subject),
			strings.TrimSpace(ref.URI),
		)
	}

	bitstring, bits, err := decodeXFSCSignedClaims(claims)
	if err != nil {
		return status.Result{}, err
	}

	return h.mapBitstringResult(ref, bitstring, bits)
}

func (h *XFSC) mapBitstringResult(ref status.Reference, bitstring []byte, bits uint) (status.Result, error) {
	width := ref.StatusSize
	if width == 0 {
		width = bits
	}
	if width == 0 {
		width = 1
	}

	value, err := codec.ReadStatusValue(bitstring, ref.Index, width, codec.LSBFirst)
	if err != nil {
		return status.Result{}, err
	}

	return status.MapIETFResult(ref, value), nil
}

// UnwrapStatusListJWTBody strips a JSON-quoted JWT string returned by XFSC statuslist-service.
func UnwrapStatusListJWTBody(body []byte) []byte {
	trimmed := strings.TrimSpace(string(body))
	if strings.HasPrefix(trimmed, `"`) {
		var quoted string
		if err := json.Unmarshal(body, &quoted); err == nil {
			quoted = strings.TrimSpace(quoted)
			if status.IsLikelyJWT([]byte(quoted)) {
				return []byte(quoted)
			}
		}
	}
	if status.IsLikelyJWT([]byte(trimmed)) {
		return []byte(trimmed)
	}
	return body
}

func (h *XFSC) verifyStatusListJWT(body []byte) (envelope.VerifiedJWT, error) {
	if !status.IsLikelyJWT(body) {
		return envelope.VerifiedJWT{}, fmt.Errorf("xfsc status list response is not a jwt")
	}
	if h.Trust == nil {
		return envelope.VerifiedJWT{}, status.ErrStatusTrustNotConfigured
	}
	verified, err := envelope.VerifyES256JWT(body, func(issuer string, token *jwt.Token) (*ecdsa.PublicKey, error) {
		// An issuer that publishes its key by certificate chain has no bundled
		// JWKS to resolve, so its status list is verified from the chain the
		// token itself carries — against the same anchors and the same
		// leaf-identifies-issuer binding the credential is held to.
		if raw, ok := token.Header["x5c"]; ok {
			key, err := sdjwt.VerificationKeyFromX5C(raw, h.Trust.X5CRoots, issuer)
			if err != nil {
				return nil, err
			}
			ecKey, ok := key.(*ecdsa.PublicKey)
			if !ok {
				return nil, fmt.Errorf("status list x5c leaf key is %T, not ECDSA", key)
			}
			return ecKey, nil
		}
		return h.Trust.ResolveECDSAPublicKey(issuer)
	})
	if err != nil {
		return envelope.VerifiedJWT{}, err
	}
	if !status.IsXFSCStatusListJWTType(verified.Header) {
		return envelope.VerifiedJWT{}, fmt.Errorf("xfsc jwt typ must be statuslist+jwt or JWT")
	}
	return verified, nil
}

func decodeXFSCSignedClaims(claims map[string]any) ([]byte, uint, error) {
	statusList, ok := claims["status_list"].(map[string]any)
	if !ok {
		return nil, 0, status.ErrStatusDecoding
	}

	bits, ok := status.ParseTokenStatusBits(statusList["bits"])
	if !ok {
		bits = 1
	}

	lst, _ := statusList["lst"].(string)
	if strings.TrimSpace(lst) == "" {
		return nil, 0, status.ErrStatusDecoding
	}

	compressed, err := codec.DecodeBase64Flexible(lst)
	if err != nil {
		return nil, 0, status.ErrStatusDecoding
	}

	bitstring, err := codec.GZIPDecompressLimited(compressed, 0)
	if err != nil {
		return nil, 0, status.ErrStatusDecompression
	}
	return bitstring, bits, nil
}
