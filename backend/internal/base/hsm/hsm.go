// Package hsm is the single PKCS#11 abstraction through which every
// private-key operation in the backend passes (DCS-IR-HI-01, DCS-NFR-SEC-02).
// Keys live inside a PKCS#11 token (SoftHSM2 in dev, a real HSM in
// production); this package never handles raw private-key material.
//
// All keys are ECDSA P-256 and all signatures use SHA-256. crypto11 returns
// ASN.1 DER for ECDSA signatures; JOSE (ES256) and COSE need the fixed-width
// r||s (64-byte) encoding instead, so SignES256 does that conversion.
package hsm

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/base64"
	"fmt"
	"math/big"
	"strings"

	"github.com/ThalesGroup/crypto11"
)

// Config holds the PKCS#11 module/token coordinates read from the environment.
type Config struct {
	ModulePath string
	TokenLabel string
	Pin        string
}

// HSM is an open PKCS#11 session against a single token.
type HSM struct {
	ctx *crypto11.Context
}

// Open connects to the configured PKCS#11 token. Callers treat a returned
// error as fatal at startup: there is deliberately no software fallback, so a
// wrong module path, token label or PIN prevents the process from becoming
// healthy (DCS-NFR-SEC-02).
func Open(cfg Config) (*HSM, error) {
	if strings.TrimSpace(cfg.ModulePath) == "" {
		return nil, fmt.Errorf("PKCS11_MODULE_PATH is required")
	}
	if strings.TrimSpace(cfg.TokenLabel) == "" {
		return nil, fmt.Errorf("PKCS11_TOKEN_LABEL is required")
	}
	if strings.TrimSpace(cfg.Pin) == "" {
		return nil, fmt.Errorf("PKCS11_PIN is required")
	}

	ctx, err := crypto11.Configure(&crypto11.Config{
		Path:       cfg.ModulePath,
		TokenLabel: cfg.TokenLabel,
		Pin:        cfg.Pin,
	})
	if err != nil {
		return nil, fmt.Errorf("open pkcs11 token %q via %q: %w", cfg.TokenLabel, cfg.ModulePath, err)
	}

	return &HSM{ctx: ctx}, nil
}

// Close releases the PKCS#11 session.
func (h *HSM) Close() error {
	if h == nil || h.ctx == nil {
		return nil
	}
	return h.ctx.Close()
}

// Signer returns the ECDSA P-256 crypto.Signer for the key with the given
// CKA_LABEL. Its Sign method returns ASN.1 DER; use SignES256 for JOSE/COSE.
func (h *HSM) Signer(label string) (crypto.Signer, error) {
	if h == nil || h.ctx == nil {
		return nil, fmt.Errorf("hsm not initialised")
	}
	label = strings.TrimSpace(label)
	if label == "" {
		return nil, fmt.Errorf("hsm key label is required")
	}
	signer, err := h.ctx.FindKeyPair(nil, []byte(label))
	if err != nil {
		return nil, fmt.Errorf("find hsm key %q: %w", label, err)
	}
	if signer == nil {
		return nil, fmt.Errorf("hsm key %q not found in token", label)
	}
	if _, ok := signer.Public().(*ecdsa.PublicKey); !ok {
		return nil, fmt.Errorf("hsm key %q is not an ECDSA key", label)
	}
	return signer, nil
}

// PublicJWK returns the public key of the given label as an EC P-256 JWK
// (kty EC, crv P-256, x/y). No private material is included.
func (h *HSM) PublicJWK(label string) (map[string]any, error) {
	signer, err := h.Signer(label)
	if err != nil {
		return nil, err
	}
	pub, ok := signer.Public().(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("hsm key %q is not an ECDSA public key", label)
	}
	return ECPublicKeyJWK(pub), nil
}

// ECPublicKeyJWK renders an ECDSA P-256 public key as a JWK map.
func ECPublicKeyJWK(pub *ecdsa.PublicKey) map[string]any {
	size := (pub.Curve.Params().BitSize + 7) / 8
	return map[string]any{
		"kty": "EC",
		"crv": "P-256",
		"x":   coordinate(pub.X, size),
		"y":   coordinate(pub.Y, size),
	}
}

// SignES256 hashes message with SHA-256, signs it with the given ECDSA signer,
// and returns the 64-byte raw r||s signature required by JOSE (ES256) and COSE.
func SignES256(signer crypto.Signer, message []byte) ([]byte, error) {
	digest := sha256.Sum256(message)
	der, err := signer.Sign(rand.Reader, digest[:], crypto.SHA256)
	if err != nil {
		return nil, fmt.Errorf("ecdsa sign: %w", err)
	}
	return ECDSADERToRaw(der, elliptic.P256())
}

// ECDSADERToRaw converts an ASN.1 DER ECDSA signature into the fixed-width
// r||s encoding (2*ceil(bits/8) bytes) used by JOSE and COSE.
func ECDSADERToRaw(der []byte, curve elliptic.Curve) ([]byte, error) {
	var parsed struct{ R, S *big.Int }
	if _, err := asn1.Unmarshal(der, &parsed); err != nil {
		return nil, fmt.Errorf("parse DER ecdsa signature: %w", err)
	}
	size := (curve.Params().BitSize + 7) / 8
	out := make([]byte, 2*size)
	parsed.R.FillBytes(out[:size])
	parsed.S.FillBytes(out[size:])
	return out, nil
}

func coordinate(v *big.Int, size int) string {
	buf := make([]byte, size)
	v.FillBytes(buf)
	return base64.RawURLEncoding.EncodeToString(buf)
}
