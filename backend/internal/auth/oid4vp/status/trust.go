package status

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/url"
	"strings"
)

// TrustConfig maps issuer identifiers to trusted public keys (JWKS).
// For XFSC signed status lists, look up the JWT iss claim—often the statuslist
// tenant host URL signed by crypto-provider Vault transit, not the credential iss.
//
// This is a key-resolution view of the trust document, not a second reading of
// it. It is built by projecting the issuers oid4vp.TrustConfig already parsed
// (NewTrustConfig), so there is one parse and one answer to which issuers this
// deployment trusts.
//
// The view is deliberately narrower than what it is projected from, and the
// narrowing is not a scoping decision: nothing here consults the source
// entry's purposes or organizations, because no purpose in the vocabulary
// grants status-list signing. Any issuer with a usable bundled key can sign a
// status list this deployment will honour. Adding that scope means adding the
// purpose first, in policy/trust.rego, not filtering here.
type TrustConfig struct {
	Issuers map[string]TrustIssuerEntry
	// X5CRoots are the anchors an x5c-bearing status list's chain must verify
	// against. An issuer that publishes its key by chain carries no bundled
	// JWKS, so without these its status list resolves to no key at all.
	X5CRoots *x509.CertPool
}

type TrustIssuerEntry struct {
	JWKS TrustJWKS
}

type TrustJWKS struct {
	Keys []map[string]any `json:"keys"`
}

// NewTrustConfig builds the key-resolution view from each issuer's raw "jwks"
// object. An issuer whose key is resolved by certificate chain carries no
// bundled JWKS and is absent from this view: its status list is verified from
// the chain in the token's own x5c header against X5CRoots, not from here.
//
// An empty view is therefore a legitimate result, not an error: a deployment
// whose status-list issuers all publish by certificate — which is what ADR-34
// makes ours — has no bundled key to put in it. Whether the config as a whole
// can resolve anything is decided where the anchors are also known; see
// ConfigureStatusListVerification.
func NewTrustConfig(issuerJWKS map[string]json.RawMessage) (*TrustConfig, error) {
	cfg := TrustConfig{Issuers: map[string]TrustIssuerEntry{}}
	for issuer, raw := range issuerJWKS {
		if len(raw) == 0 {
			continue
		}
		var jwks TrustJWKS
		if err := json.Unmarshal(raw, &jwks); err != nil {
			return nil, fmt.Errorf("trust config: issuer %q jwks: %w", issuer, err)
		}
		if len(jwks.Keys) == 0 {
			continue
		}
		cfg.Issuers[issuer] = TrustIssuerEntry{JWKS: jwks}
	}
	return &cfg, nil
}

func (t *TrustConfig) ResolveECDSAPublicKey(issuer string) (*ecdsa.PublicKey, error) {
	entry, ok := t.Issuers[issuer]
	if !ok {
		return nil, fmt.Errorf("issuer %q not in trust list", issuer)
	}
	for _, key := range entry.JWKS.Keys {
		pub, err := jwkToECDSAPublicKey(key)
		if err == nil {
			return pub, nil
		}
	}
	return nil, fmt.Errorf("no usable EC key for issuer %q", issuer)
}

// ResolveECDSAPublicKeyByKID looks up an EC P-256 key by JWK kid within the trust scope
// of statusListURI. COSE kid values are not globally unique; URI binding is always required.
func (t *TrustConfig) ResolveECDSAPublicKeyByKID(statusListURI, kid string) (*ecdsa.PublicKey, error) {
	kid = strings.TrimSpace(kid)
	if kid == "" {
		return nil, fmt.Errorf("empty COSE kid")
	}
	statusListURI = strings.TrimSpace(statusListURI)
	if statusListURI == "" {
		return nil, fmt.Errorf("status list URI is required for COSE kid lookup")
	}

	type candidate struct {
		issuer string
		pub    *ecdsa.PublicKey
	}
	var scoped []candidate
	for issuer, entry := range t.Issuers {
		if !issuerTrustedForStatusListURI(issuer, statusListURI) {
			continue
		}
		for _, key := range entry.JWKS.Keys {
			keyKid, _ := key["kid"].(string)
			if strings.TrimSpace(keyKid) != kid {
				continue
			}
			pub, err := jwkToECDSAPublicKey(key)
			if err != nil {
				continue
			}
			scoped = append(scoped, candidate{issuer: issuer, pub: pub})
		}
	}

	switch len(scoped) {
	case 0:
		return nil, fmt.Errorf("kid %q is not trusted for status list URI %q", kid, statusListURI)
	case 1:
		return scoped[0].pub, nil
	default:
		return nil, fmt.Errorf("kid %q matches multiple trusted keys for status list URI %q", kid, statusListURI)
	}
}

func issuerTrustedForStatusListURI(issuer, statusListURI string) bool {
	issuer = strings.TrimSpace(issuer)
	statusListURI = strings.TrimSpace(statusListURI)
	if issuer == "" || statusListURI == "" {
		return false
	}
	if SubjectMatchesURI(issuer, statusListURI) {
		return true
	}

	issuerURL, err := url.Parse(issuer)
	if err != nil || issuerURL.Host == "" {
		return false
	}
	listURL, err := url.Parse(statusListURI)
	if err != nil || listURL.Host == "" {
		return false
	}
	return strings.EqualFold(issuerURL.Scheme, listURL.Scheme) &&
		strings.EqualFold(issuerURL.Host, listURL.Host)
}

func (t *TrustConfig) ResolveEd25519PublicKey(issuer string) (ed25519.PublicKey, error) {
	entry, ok := t.Issuers[issuer]
	if !ok {
		return nil, fmt.Errorf("issuer %q not in trust list", issuer)
	}
	for _, key := range entry.JWKS.Keys {
		pub, err := jwkToEd25519PublicKey(key)
		if err == nil {
			return pub, nil
		}
	}
	return nil, fmt.Errorf("no usable Ed25519 key for issuer %q", issuer)
}

func jwkToECDSAPublicKey(jwk map[string]any) (*ecdsa.PublicKey, error) {
	kty, _ := jwk["kty"].(string)
	crv, _ := jwk["crv"].(string)
	if kty != "EC" || crv != "P-256" {
		return nil, fmt.Errorf("unsupported key type")
	}
	xRaw, _ := jwk["x"].(string)
	yRaw, _ := jwk["y"].(string)
	x, err := decodeBase64URLInt(xRaw)
	if err != nil {
		return nil, err
	}
	y, err := decodeBase64URLInt(yRaw)
	if err != nil {
		return nil, err
	}
	return &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}, nil
}

func jwkToEd25519PublicKey(jwk map[string]any) (ed25519.PublicKey, error) {
	kty, _ := jwk["kty"].(string)
	crv, _ := jwk["crv"].(string)
	if kty != "OKP" || crv != "Ed25519" {
		return nil, fmt.Errorf("unsupported key type")
	}
	xRaw, _ := jwk["x"].(string)
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(xRaw))
	if err != nil {
		return nil, err
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid ed25519 public key length")
	}
	pub := make(ed25519.PublicKey, ed25519.PublicKeySize)
	copy(pub, raw)
	return pub, nil
}

func decodeBase64URLInt(s string) (*big.Int, error) {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return nil, err
	}
	return new(big.Int).SetBytes(raw), nil
}
