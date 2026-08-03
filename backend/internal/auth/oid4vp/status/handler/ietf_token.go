package handler

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"time"

	"digital-contracting-service/internal/auth/oid4vp/sdjwt"
	"digital-contracting-service/internal/auth/oid4vp/status"
	"digital-contracting-service/internal/auth/oid4vp/status/codec"
	"digital-contracting-service/internal/auth/oid4vp/status/envelope"
	"digital-contracting-service/internal/auth/oid4vp/status/fetch"

	"github.com/golang-jwt/jwt/v5"
)

type IETFToken struct {
	Fetcher *fetch.Client
	Trust   *status.TrustConfig
	Now     func() time.Time
}

func (h *IETFToken) Mechanism() status.Mechanism {
	return status.MechanismIETFToken
}

func (h *IETFToken) Check(
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
		response, err = status.FetchStatusList(ctx, h.Fetcher, ref.URI, fetch.RequestOpts{
			Accept: status.IETFStatusListAccept,
		})
		if err != nil {
			return status.Result{}, status.ErrStatusRetrieval
		}
	}

	contentType := envelope.NormalizeContentType(response.ContentType)
	body := response.Body

	var claims map[string]any
	switch contentType {
	case "application/statuslist+jwt":
		verified, err := h.verifyJWT(body)
		if err != nil {
			return status.Result{}, status.ErrStatusSignature
		}
		if err := status.ValidateIETFStatusListJWTHeader(verified.Header); err != nil {
			return status.Result{}, status.ErrStatusSignature
		}
		claims = verified.Claims
	case "application/statuslist+cwt":
		var err error
		claims, err = h.verifyCWT(body, ref.URI)
		if err != nil {
			return status.Result{}, status.ErrStatusSignature
		}
	default:
		if status.LooksLikeJSON(body) {
			return status.Result{}, status.ErrStatusListNotSecured
		}
		return status.Result{}, status.ErrUnsupportedMediaType
	}

	subject, _ := claims["sub"].(string)
	if subject != ref.URI {
		return status.Result{}, status.ErrStatusURIMismatch
	}

	if err := h.validateTimeClaims(claims); err != nil {
		return status.Result{}, err
	}

	if normalized, ok := status.NormalizeAnyMap(claims); ok {
		claims = normalized
	}

	statusList, ok := claims["status_list"].(map[string]any)
	if !ok {
		return status.Result{}, status.ErrStatusDecoding
	}

	bits, ok := status.ParseTokenStatusBits(statusList["bits"])
	if !ok {
		return status.Result{}, status.ErrInvalidStatusSize
	}

	compressed, err := h.decodeIETFList(statusList["lst"], contentType)
	if err != nil {
		return status.Result{}, status.ErrStatusDecoding
	}

	bitstring, err := codec.ZLIBDecompressLimited(compressed, 0)
	if err != nil {
		return status.Result{}, status.ErrStatusDecompression
	}

	value, err := codec.ReadStatusValue(bitstring, ref.Index, bits, codec.LSBFirst)
	if err != nil {
		if errors.Is(err, codec.ErrIndexOutOfRange) {
			return status.Result{}, status.ErrIndexOutOfRange
		}
		return status.Result{}, err
	}

	return status.MapIETFResult(ref, value), nil
}

func (h *IETFToken) decodeIETFList(lst any, contentType string) ([]byte, error) {
	if contentType == "application/statuslist+cwt" {
		return envelope.DecodeCWTListBytes(lst)
	}
	raw, _ := lst.(string)
	return codec.DecodeBase64URL(raw)
}

func (h *IETFToken) verifyJWT(body []byte) (envelope.VerifiedJWT, error) {
	if err := requireStatusTrust(h.Trust); err != nil {
		return envelope.VerifiedJWT{}, err
	}
	return envelope.VerifyES256JWT(body, func(issuer string, token *jwt.Token) (*ecdsa.PublicKey, error) {
		// An issuer that publishes its key by certificate chain bundles no JWKS,
		// so its status list is verified from the chain the token carries —
		// against the same anchors and the same leaf-identifies-issuer binding
		// the credential is held to. This is the path a statuslist+jwt from the
		// issuer's own flow takes: SelectMechanismFromResponse routes that
		// content type here, not to the XFSC handler.
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
}

func (h *IETFToken) verifyCWT(body []byte, statusListURI string) (map[string]any, error) {
	if err := requireStatusTrust(h.Trust); err != nil {
		return nil, err
	}
	return envelope.VerifyStatusListCWT(body, envelope.CWTVerifier{
		StatusListURI: statusListURI,
		ResolveECDSAByKID: func(uri, kid string) (*ecdsa.PublicKey, error) {
			return h.Trust.ResolveECDSAPublicKeyByKID(uri, kid)
		},
		ResolveECDSA: h.Trust.ResolveECDSAPublicKey,
	})
}

func (h *IETFToken) validateTimeClaims(claims map[string]any) error {
	now := time.Now().UTC()
	if h.Now != nil {
		now = h.Now()
	}
	if exp, ok := envelope.ExpFromClaims(claims); ok && now.Unix() >= exp {
		return fmt.Errorf("status list token expired")
	}
	if nbf, ok := envelope.NbfFromClaims(claims); ok && now.Unix() < nbf {
		return fmt.Errorf("status list token not yet valid")
	}
	return nil
}
