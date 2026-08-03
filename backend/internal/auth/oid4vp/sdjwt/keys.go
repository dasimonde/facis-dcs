package sdjwt

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// minRSAKeyBits is the smallest RSA issuer key accepted from an x5c leaf.
const minRSAKeyBits = 2048

// JWK is an EC P-256 public key used for SD-JWT verification.
type JWK struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
	D   string `json:"d,omitempty"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	Use string `json:"use"`
}

type jwksDocument struct {
	Keys []JWK `json:"keys"`
}

// TrustConfig provides issuer trust queries used during JWT signature verification.
type TrustConfig interface {
	IssuerTrusted(iss string) bool
	VCTAllowed(vct string) bool
	IssuerJWKS(iss string) (json.RawMessage, error)
	// IssuerUsesX5C reports whether this issuer publishes its key through a
	// certificate chain. It decides which resolution branch may run, so the
	// CONFIGURATION picks the path and not the credential: a credential that
	// arrives with an x5c header for an issuer configured to publish a JWKS is
	// presenting a key from somewhere the operator never trusted.
	IssuerUsesX5C(iss string) (bool, error)
	// X5CTrustRoots returns the trust anchors an x5c-bearing credential's
	// certificate chain must verify against, or nil if none are configured —
	// in which case an x5c-bearing credential must be refused outright.
	X5CTrustRoots() *x509.CertPool
}

// --- Issuer credential JWT: verification key resolution ---

// ResolveIssuerVerificationKey returns the public key used to verify a credential issuer JWT.
//
// Trust and key material are resolved inside the JWT keyfunc so verification never proceeds
// with an untrusted or unknown issuer key. Resolution order:
//
// The issuer's CONFIGURED mechanism selects the branch. Letting the credential
// choose — x5c header present, therefore validate a chain — would mean an
// attacker holding any certificate under any configured anchor could present it
// for an issuer whose keys are published as a JWKS, and be believed.
//
//  1. issuer publishes via x5c — the chain in the header, verified to the anchors.
//  2. otherwise — header.jwk matched against the issuer's resolved JWKS, or
//     header.kid looked up in it.
func ResolveIssuerVerificationKey(cfg TrustConfig, token *jwt.Token) (any, error) {
	if cfg == nil {
		return nil, fmt.Errorf("trust config is not configured")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("credential jwt claims are invalid")
	}

	iss, _ := claims["iss"].(string)
	if strings.TrimSpace(iss) == "" {
		return nil, fmt.Errorf("credential jwt missing iss")
	}
	if !cfg.IssuerTrusted(iss) {
		return nil, fmt.Errorf("issuer %q is not trusted", iss)
	}

	usesX5C, err := cfg.IssuerUsesX5C(iss)
	if err != nil {
		return nil, err
	}

	if usesX5C {
		rawX5C, ok := token.Header["x5c"]
		if !ok {
			return nil, fmt.Errorf("issuer %q publishes its key through a certificate chain, but the credential carries no x5c header", iss)
		}
		return verificationKeyFromX5C(rawX5C, cfg.X5CTrustRoots(), iss)
	}

	if _, ok := token.Header["x5c"]; ok {
		return nil, fmt.Errorf("credential for issuer %q carries an x5c header, but that issuer publishes its key another way; the chain proves nothing about it", iss)
	}

	jwksRaw, err := cfg.IssuerJWKS(iss)
	if err != nil {
		return nil, err
	}

	if rawJWK, ok := token.Header["jwk"]; ok {
		return verificationKeyFromHeaderJWK(jwksRaw, rawJWK)
	}

	return verificationKeyFromTrustedJWKS(jwksRaw, token, iss)
}

// ResolveIssuerVerificationKeyForPID resolved a PID issuer's key from the
// credential's own certificate chain without ever asking whether that issuer was
// trusted — no iss, no trust lookup, no purpose. Any certificate under any
// configured anchor could therefore sign a PID for any issuer, which is the
// relying-party-attests-to-itself hazard the purpose split exists to prevent.
//
// PID now resolves through ResolveIssuerVerificationKey like everything else, so
// it inherits the trust and purpose checks instead of bypassing them.

// VerificationKeyFromX5C resolves a verification key from a JWS x5c header for
// a caller outside this package. A status list is signed by the same issuer as
// the credential whose status it carries, so it is verified the same way and
// under the same anchors — including the leaf-identifies-issuer binding, without
// which any certificate under any configured anchor could sign any issuer's
// status list.
func VerificationKeyFromX5C(raw any, roots *x509.CertPool, iss string) (any, error) {
	return verificationKeyFromX5C(raw, roots, iss)
}

// verificationKeyFromX5C parses the full x5c chain (leaf first, per RFC 7517
// §4.7), verifies leaf -> intermediates -> roots, and returns the leaf's
// public key. roots being nil (no trust anchors configured) is refused, not
// silently accepted off the chain's own say-so — an x5c header proves
// nothing about WHO the leaf belongs to without a trust anchor to verify
// against; trusting an unverified chain would let anyone mint their own
// key+cert and self-certify as any issuer.
func verificationKeyFromX5C(raw any, roots *x509.CertPool, iss string) (any, error) {
	if roots == nil {
		return nil, fmt.Errorf("no x5c trust anchors are configured")
	}

	certsRaw, ok := raw.([]any)
	if !ok || len(certsRaw) == 0 {
		return nil, fmt.Errorf("x5c header is empty")
	}

	certs := make([]*x509.Certificate, 0, len(certsRaw))
	for i, entry := range certsRaw {
		certB64, ok := entry.(string)
		if !ok || strings.TrimSpace(certB64) == "" {
			return nil, fmt.Errorf("x5c[%d] is invalid", i)
		}
		der, err := base64.StdEncoding.DecodeString(certB64)
		if err != nil {
			return nil, fmt.Errorf("decode x5c[%d]: %w", i, err)
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return nil, fmt.Errorf("parse x5c[%d]: %w", i, err)
		}
		certs = append(certs, cert)
	}

	leaf := certs[0]
	intermediates := x509.NewCertPool()
	for _, cert := range certs[1:] {
		intermediates.AddCert(cert)
	}

	if _, err := leaf.Verify(x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}); err != nil {
		return nil, fmt.Errorf("x5c certificate chain does not verify against configured trust anchors: %w", err)
	}

	// A chain proves the anchor vouched for this certificate. It does NOT say
	// the certificate belongs to the issuer the credential names — without this
	// check any certificate under any configured anchor, including a TLS server
	// certificate, signs credentials asserting any issuer identity.
	binding, err := leafIdentifiesIssuer(leaf, iss)
	if err != nil {
		return nil, err
	}

	if err := leafMayAttest(leaf, binding); err != nil {
		return nil, err
	}

	return leafVerificationKey(leaf)
}

// leafVerificationKey returns the leaf's public key, refusing key types and
// sizes no issuer should be signing credentials with. Real PID and QTSP issuer
// certificates are routinely RSA or a curve above P-256, so the accepted set is
// what JWS can verify at an adequate strength rather than the single curve this
// deployment's own issuer happens to use.
func leafVerificationKey(leaf *x509.Certificate) (any, error) {
	switch pk := leaf.PublicKey.(type) {
	case *ecdsa.PublicKey:
		switch pk.Curve {
		case elliptic.P256(), elliptic.P384(), elliptic.P521():
			return pk, nil
		}
		return nil, fmt.Errorf("x5c leaf certificate uses unsupported curve %q", pk.Curve.Params().Name)
	case *rsa.PublicKey:
		if pk.N.BitLen() < minRSAKeyBits {
			return nil, fmt.Errorf("x5c leaf certificate rsa key is %d bits, below the %d-bit minimum", pk.N.BitLen(), minRSAKeyBits)
		}
		return pk, nil
	default:
		return nil, fmt.Errorf("x5c leaf certificate public key type %T cannot verify a JWS", leaf.PublicKey)
	}
}

func verificationKeyFromHeaderJWK(jwksRaw json.RawMessage, rawJWK any) (any, error) {
	headerKey, err := JWKFromAny(rawJWK)
	if err != nil {
		return nil, err
	}

	err = assertJWKTrusted(jwksRaw, headerKey)
	if err != nil {
		return nil, err
	}

	return ecPublicKey(headerKey.X, headerKey.Y)
}

func verificationKeyFromTrustedJWKS(jwksRaw json.RawMessage, token *jwt.Token, iss string) (any, error) {
	var doc jwksDocument
	err := json.Unmarshal(jwksRaw, &doc)

	if err != nil {
		return nil, fmt.Errorf("parse issuer jwks: %w", err)
	}

	kid, _ := token.Header["kid"].(string)
	if kid == "" {
		// Without a kid the key choice must be unambiguous.
		if len(doc.Keys) != 1 {
			return nil, fmt.Errorf("credential jwt has no kid and issuer jwks has %d keys", len(doc.Keys))
		}
		return trustedECKey(doc.Keys[0])
	}

	for _, key := range doc.Keys {
		if kidNamesKey(key.Kid, kid, iss) {
			return trustedECKey(key)
		}
	}

	return nil, fmt.Errorf("no matching issuer jwk for kid %q", kid)
}

// kidNamesKey reports whether a credential's kid names the key a JWKS entry
// carries.
//
// A DID document publishes two names for one key: the verification method's DID
// URL (did:web:host#dev-key-1) and the JWK's own kid inside it (dev-key-1) —
// this repository's gendid writes both. An issuer may sign under either, so
// comparing the strings alone leaves the issuer and the verifier looking each
// other up by different names. A DID URL therefore also matches the bare
// fragment it ends in.
//
// A DID URL only names a key of the document it points at, so its base must be
// the issuer whose keys were resolved. Matching on the fragment alone would let
// a credential cite another controller's document and still be verified here.
func kidNamesKey(jwkKid, credentialKid, iss string) bool {
	if jwkKid == credentialKid {
		return true
	}
	fragmentOf := func(didURL string) string {
		base, fragment, found := strings.Cut(didURL, "#")
		if !found || fragment == "" || base != iss {
			return ""
		}
		return fragment
	}
	switch {
	case strings.Contains(jwkKid, "#") && !strings.Contains(credentialKid, "#"):
		return credentialKid != "" && fragmentOf(jwkKid) == credentialKid
	case strings.Contains(credentialKid, "#") && !strings.Contains(jwkKid, "#"):
		return jwkKid != "" && fragmentOf(credentialKid) == jwkKid
	}
	return false
}

func trustedECKey(key JWK) (any, error) {
	if key.Kty != "EC" || key.Crv != "P-256" {
		return nil, fmt.Errorf("issuer jwk %q is not an EC P-256 key", key.Kid)
	}

	return ecPublicKey(key.X, key.Y)
}

func assertJWKTrusted(jwksRaw json.RawMessage, candidate JWK) error {
	var doc jwksDocument
	err := json.Unmarshal(jwksRaw, &doc)

	if err != nil {
		return fmt.Errorf("parse issuer jwks: %w", err)
	}

	for _, trusted := range doc.Keys {
		if publicJWKsEqual(candidate, trusted) {
			return nil
		}
	}

	return fmt.Errorf("credential issuer jwk is not trusted")
}

// --- Holder KB-JWT: verification key ---

func holderVerificationKey(cnfJWK JWK, token *jwt.Token) (any, error) {
	_ = token

	return ecPublicKey(cnfJWK.X, cnfJWK.Y)
}

// --- JWK primitives ---

// JWKFromAny parses a JWK from a JWT header or claim value.
func JWKFromAny(raw any) (JWK, error) {
	switch v := raw.(type) {
	case map[string]any:
		return ecP256PublicKeyFromMap(v)
	case JWK:
		return ecP256PublicKeyFromJWK(v)
	default:
		return JWK{}, fmt.Errorf("unsupported jwk value")
	}
}

// DIDJWKFromPublicJWK builds a did:jwk identifier from an EC public JWK.
func DIDJWKFromPublicJWK(key JWK) (string, error) {
	if strings.TrimSpace(key.D) != "" {
		return "", fmt.Errorf("did:jwk must not include private key")
	}

	public, err := ecP256PublicKeyFromJWK(key)
	if err != nil {
		return "", err
	}

	payload, err := json.Marshal(map[string]string{
		"crv": public.Crv,
		"kty": public.Kty,
		"x":   public.X,
		"y":   public.Y,
	})
	if err != nil {
		return "", err
	}

	return "did:jwk:" + base64.RawURLEncoding.EncodeToString(payload), nil
}

// JWKFromDIDJWK decodes a did:jwk identifier into public-key material.
func JWKFromDIDJWK(did string) (JWK, error) {
	did = strings.TrimSpace(did)
	if !strings.HasPrefix(did, "did:jwk:") {
		return JWK{}, fmt.Errorf("subject is not a did:jwk identifier")
	}

	encoded := strings.TrimPrefix(did, "did:jwk:")
	if encoded == "" {
		return JWK{}, fmt.Errorf("did:jwk payload is empty")
	}

	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return JWK{}, fmt.Errorf("decode did:jwk payload: %w", err)
	}

	var payload map[string]any
	err = json.Unmarshal(raw, &payload)
	if err != nil {
		return JWK{}, fmt.Errorf("parse did:jwk payload: %w", err)
	}

	return ecP256PublicKeyFromMap(payload)
}

// HolderSubject returns the identifier of the holder a credential is bound to.
//
// SD-JWT VC makes `sub` OPTIONAL and its value arbitrary — the holder binding
// is `cnf`. A credential that names no subject is therefore identified by the
// did:jwk of its binding key, one that names a did:jwk must name that same key,
// and any other identifier is the issuer's to choose.
func HolderSubject(claims jwt.MapClaims) (string, error) {
	cnfJWK, err := CNFJWKFromClaims(claims)
	if err != nil {
		return "", err
	}

	sub, _ := claims["sub"].(string)
	sub = strings.TrimSpace(sub)

	if sub == "" {
		bindingDID, err := DIDJWKFromPublicJWK(cnfJWK)
		if err != nil {
			return "", fmt.Errorf("credential cnf.jwk: %w", err)
		}
		return bindingDID, nil
	}

	if strings.HasPrefix(sub, "did:jwk:") {
		if err := HolderSubjectMatches(sub, cnfJWK); err != nil {
			return "", err
		}
	}

	return sub, nil
}

// HolderSubjectMatches reports whether credential sub and cnf.jwk identify the same holder key.
func HolderSubjectMatches(sub string, cnfJWK JWK) error {
	sub = strings.TrimSpace(sub)
	if sub == "" {
		return fmt.Errorf("credential missing sub")
	}

	cnf, err := ecP256PublicKeyFromJWK(cnfJWK)
	if err != nil {
		return fmt.Errorf("credential cnf.jwk: %w", err)
	}

	subject, err := JWKFromDIDJWK(sub)
	if err != nil {
		return fmt.Errorf("credential sub: %w", err)
	}

	if !publicJWKsEqual(subject, cnf) {
		return fmt.Errorf("credential sub does not match cnf.jwk holder binding")
	}

	return nil
}

func ecP256PublicKeyFromMap(raw map[string]any) (JWK, error) {
	return ecP256PublicKeyFromJWK(JWK{
		Kty: stringValue(raw["kty"]),
		Crv: stringValue(raw["crv"]),
		X:   stringValue(raw["x"]),
		Y:   stringValue(raw["y"]),
	})
}

func ecP256PublicKeyFromJWK(key JWK) (JWK, error) {
	key.Kty = strings.TrimSpace(key.Kty)
	key.Crv = strings.TrimSpace(key.Crv)
	key.X = strings.TrimSpace(key.X)
	key.Y = strings.TrimSpace(key.Y)

	if key.Kty != "EC" {
		return JWK{}, fmt.Errorf("unsupported jwk kty %q", key.Kty)
	}
	if key.Crv == "" {
		key.Crv = "P-256"
	}
	if key.Crv != "P-256" {
		return JWK{}, fmt.Errorf("unsupported jwk crv %q", key.Crv)
	}
	if key.X == "" || key.Y == "" {
		return JWK{}, fmt.Errorf("jwk is missing public key material")
	}

	return key, nil
}

func publicJWKsEqual(a, b JWK) bool {
	aNorm, errA := ecP256PublicKeyFromJWK(a)
	bNorm, errB := ecP256PublicKeyFromJWK(b)
	if errA != nil || errB != nil {
		return false
	}

	return aNorm.Kty == bNorm.Kty &&
		aNorm.Crv == bNorm.Crv &&
		aNorm.X == bNorm.X &&
		aNorm.Y == bNorm.Y
}

func stringValue(v any) string {
	s, _ := v.(string)

	return strings.TrimSpace(s)
}

func ecPublicKey(xB64, yB64 string) (*ecdsa.PublicKey, error) {
	x, err := decodeCoordinate(xB64)

	if err != nil {
		return nil, err
	}

	y, err := decodeCoordinate(yB64)

	if err != nil {
		return nil, err
	}

	return &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}, nil
}

func decodeCoordinate(value string) (*big.Int, error) {
	raw, err := base64.RawURLEncoding.DecodeString(value)

	if err != nil {
		return nil, err
	}

	return new(big.Int).SetBytes(raw), nil
}

// issuerBinding is how a leaf certificate was tied to the issuer identifier it
// speaks for.
type issuerBinding int

const (
	bindingNone issuerBinding = iota
	// bindingURI: a SAN URI holding the issuer identifier verbatim.
	bindingURI
	// bindingDNS: a SAN DNS name matching the identifier's authority. This is
	// the only binding an ordinary TLS certificate for the issuer's host also
	// satisfies.
	bindingDNS
	// bindingCN: a subject common name equal to the issuer identifier.
	bindingCN
)

// leafIdentifiesIssuer requires the certificate to carry the issuer identity it
// is being used to speak for, as a SAN URI, a SAN DNS name matching the
// identifier's authority, or failing both an exactly matching subject CN, and
// reports which of those established it.
func leafIdentifiesIssuer(leaf *x509.Certificate, iss string) (issuerBinding, error) {
	iss = strings.TrimSpace(iss)
	if iss == "" {
		return bindingNone, fmt.Errorf("x5c leaf cannot be bound to an empty issuer")
	}

	for _, uri := range leaf.URIs {
		if uri.String() == iss {
			return bindingURI, nil
		}
	}

	authority := issuerAuthority(iss)
	if authority != "" {
		for _, name := range leaf.DNSNames {
			if strings.EqualFold(name, authority) {
				return bindingDNS, nil
			}
		}
	}

	if leaf.Subject.CommonName == iss {
		return bindingCN, nil
	}

	return bindingNone, fmt.Errorf("x5c leaf certificate (subject %q, dns %v, uris %v) does not identify issuer %q",
		leaf.Subject.CommonName, leaf.DNSNames, leaf.URIs, iss)
}

// leafMayAttest refuses a certificate whose own extensions say it was issued
// for something other than signing.
//
// The chain is verified with ExtKeyUsageAny, so nothing else looks at usage.
// The hazard is an anchor that also issues TLS certificates: a server
// certificate for the issuer's own host satisfies the DNS-name binding, and
// nothing else would tell it apart from a credential signer. So TLS-only
// extended key usage is refused for exactly that binding. A certificate that
// names the issuer identifier itself — a SAN URI or a matching CN — is not a
// server certificate for that host regardless of its EKUs, and many real issuer
// certificates come out of web PKI carrying serverAuth and clientAuth.
func leafMayAttest(leaf *x509.Certificate, binding issuerBinding) error {
	if leaf.KeyUsage != 0 && leaf.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		return fmt.Errorf("x5c leaf certificate does not permit digital signatures")
	}

	if binding != bindingDNS || len(leaf.ExtKeyUsage) == 0 {
		return nil
	}

	for _, usage := range leaf.ExtKeyUsage {
		if usage != x509.ExtKeyUsageServerAuth && usage != x509.ExtKeyUsageClientAuth {
			return nil
		}
	}

	return fmt.Errorf("x5c leaf certificate is a TLS certificate (extended key usage %v) identified only by a dns name, which does not distinguish it from a server certificate for the issuer's host", leaf.ExtKeyUsage)
}

// issuerAuthority is the host an issuer identifier belongs to, for both the
// https:// and did:web forms a deployment may use.
func issuerAuthority(iss string) string {
	if rest, ok := strings.CutPrefix(iss, "did:web:"); ok {
		authority := strings.Split(rest, ":")[0]
		return strings.ReplaceAll(authority, "%3A", ":")
	}
	if rest, ok := strings.CutPrefix(iss, "https://"); ok {
		return strings.Split(rest, "/")[0]
	}
	return ""
}
