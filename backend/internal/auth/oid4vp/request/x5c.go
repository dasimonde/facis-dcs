package request

import (
	"crypto/elliptic"
	"fmt"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"digital-contracting-service/internal/base/hsm"
	"digital-contracting-service/internal/base/identity"
)

// X5CSigner signs every OpenID4VP request object (JAR) this deployment issues
// — login, PID, the signing ceremony's identity presentation and its
// Document-Retrieval request — with the DCS's own DID/hostname certificate
// chain in the header (x5c) rather than a bare jwk. The x509_san_dns client
// identifier requires this: a real wallet resolves trust from the leaf
// certificate's SAN — which must equal client_id — not from an out-of-band key
// lookup keyed by kid, and a bare jwk anchors to nothing it knows.
// It reuses the same HSM-backed key and eIDAS-shaped certificate chain
// already published at /.well-known/did.json and used for DCS-to-DCS JAdES
// sync (jades.Sign) — the DCS attesting as itself, never as a contracting party.
type X5CSigner struct {
	did *identity.DIDDocument
}

// NewX5CSigner builds an x509_san_dns JAR signer over the DCS's own DID
// document identity.
func NewX5CSigner(did *identity.DIDDocument) (*X5CSigner, error) {
	if did == nil {
		return nil, fmt.Errorf("did document is required for x5c JAR signing")
	}
	method := did.SigningMethod()
	if method == nil {
		return nil, fmt.Errorf("did document is not bound to a signer")
	}
	if len(method.PublicKeyJWK.X5C) == 0 {
		return nil, fmt.Errorf("verification method %q carries no x5c certificate chain", method.ID)
	}
	return &X5CSigner{did: did}, nil
}

// ClientID returns the complete OpenID4VP client identifier every request
// object this signer signs must declare — prefix and the DNS hostname the
// signer's own certificate identifies (VerifyEIDASCertificate already asserts
// the leaf matches it). It is the sole source of that identifier, so the deep
// link, the identity request object, the Document-Retrieval request object and
// the audience a presentation is checked against cannot name the verifier
// differently.
func (s *X5CSigner) ClientID() (string, error) {
	if s == nil || s.did == nil {
		return "", fmt.Errorf("x5c signer is not configured")
	}
	hostname, err := s.did.GetHostname()
	if err != nil {
		return "", err
	}
	clientID := X509SANDNSClientID(hostname)
	if clientID == "" {
		return "", fmt.Errorf("did document hostname %q carries no dns name", hostname)
	}
	return clientID, nil
}

// SignAuthorizationRequestJWT returns a compact oauth-authz-req+jwt signed by
// the DID document's HSM key, with the x5c certificate chain embedded in the
// header instead of a bare jwk.
func (s *X5CSigner) SignAuthorizationRequestJWT(claims jwt.MapClaims) (string, error) {
	if s == nil || s.did == nil {
		return "", fmt.Errorf("x5c signer is not configured")
	}
	// kid and chain both describe the key that actually signs below: the method
	// the document publishes this instance's signer as.
	method := s.did.SigningMethod()
	if method == nil {
		return "", fmt.Errorf("did document is not bound to a signer")
	}
	kid := method.ID
	extraHeaders := map[string]any{"x5c": []string(method.PublicKeyJWK.X5C)}

	return signES256JWT(kid, claims, extraHeaders, func(signingInput string) ([]byte, error) {
		der, err := s.did.Sign([]byte(signingInput))
		if err != nil {
			return nil, fmt.Errorf("x5c jar signing failed: %w", err)
		}
		return hsm.ECDSADERToRaw(der, elliptic.P256())
	})
}

// X509SANDNSClientPrefix is the OpenID4VP client identifier prefix for a
// verifier that proves itself with an X.509 certificate whose SAN carries the
// DNS name it claims.
const X509SANDNSClientPrefix = "x509_san_dns"

// X509SANDNSClientID renders the client identifier a wallet is given. The
// prefix is part of the identifier, not a separate parameter: a bare value is
// read as the "pre-registered" prefix, which means "you already know me out of
// band" and is refused by any wallet that has no such prior arrangement.
func X509SANDNSClientID(hostname string) string {
	host := dnsNameOf(hostname)
	if host == "" {
		return ""
	}
	return X509SANDNSClientPrefix + ":" + host
}

// dnsNameOf reduces a hostname, or an already-rendered client identifier, to
// the bare dNSName a certificate SAN can hold. A dNSName holds a name, never a
// port, so an identifier carrying one can match no certificate: deployments
// reached on a non-default port — dev and the test cluster — would otherwise
// claim a hostname their own certificate cannot back, and a wallet refuses
// exactly that.
func dnsNameOf(hostname string) string {
	// Strip any prefix first: what follows it is the name, and the prefix's own
	// colon must not be mistaken for a port separator below.
	host := strings.TrimPrefix(strings.TrimSpace(hostname), X509SANDNSClientPrefix+":")
	if idx := strings.LastIndex(host, ":"); idx > 0 {
		host = host[:idx]
	}
	return host
}
