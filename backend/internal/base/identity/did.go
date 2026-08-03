// Package identity implements the did:web-based peer identity and trust
// model used for DCS-to-DCS federation (see dcstodcs, contractworkflowengine
// /remotesync). Each DCS instance publishes its own DID document (ECDSA P-256
// key pair, held in the PKCS#11 token) at /.well-known/did.json. Trust between
// two independently operated
// instances rests on three layers, all implemented in this file: (1) an
// eIDAS certificate chain in the DID document, and (2) a per-request
// challenge-response signature proving possession of the private key, used
// instead of a shared token since there is no common auth authority across
// operators — both applied to ONE key, the one the peer publishes for
// authenticating itself and which answered the challenge (VerifyPeerChallenge);
// and (3) the federation trust gate — a self-signed agreement
// credential plus a local policy endpoint (see dcstodcs.TrustGate, ADR-19) —
// which is deliberately not part of this package.
package identity

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"digital-contracting-service/internal/base/safehttp"
)

// eIDAS / ETSI EN 319 412-5 OIDs
var (
	oidQCStatements = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 1, 3} // id-pe-qcStatements
	oidQcCompliance = asn1.ObjectIdentifier{0, 4, 0, 1862, 1, 1}       // esi4-qcStatement-1: qualified certificate
	oidQcSSCD       = asn1.ObjectIdentifier{0, 4, 0, 1862, 1, 4}       // esi4-qcStatement-4: QSCD
)

type qcStatement struct {
	StatementID   asn1.ObjectIdentifier
	StatementInfo asn1.RawValue `asn1:"optional"`
}

// X5C is the x5c certificate chain of a JWK (RFC 7517 §4.7).
// During unmarshaling it accepts both an array of strings and a single
// string and normalizes to []string.
type X5C []string

func (x *X5C) UnmarshalJSON(data []byte) error {
	var arr []string
	if err := json.Unmarshal(data, &arr); err == nil {
		*x = arr
		return nil
	}

	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*x = X5C{s}
		return nil
	}

	return fmt.Errorf("x5c: expected string or array of strings, got %s", string(data))
}

type PublicKeyJWK struct {
	Kty string `json:"kty"`
	Crv string `json:"crv,omitempty"`
	X   string `json:"x,omitempty"`
	Y   string `json:"y,omitempty"`
	X5C X5C    `json:"x5c,omitempty"`
}

// ECPublicKey builds an *ecdsa.PublicKey from the JWK fields crv, x and y.
func (jwk PublicKeyJWK) ECPublicKey() (*ecdsa.PublicKey, error) {
	if jwk.Crv != "" && jwk.Crv != "P-256" {
		return nil, fmt.Errorf("unsupported EC curve %q (only P-256 is supported)", jwk.Crv)
	}
	xBytes, err := base64.RawURLEncoding.DecodeString(jwk.X)
	if err != nil {
		return nil, fmt.Errorf("decoding x: %w", err)
	}
	yBytes, err := base64.RawURLEncoding.DecodeString(jwk.Y)
	if err != nil {
		return nil, fmt.Errorf("decoding y: %w", err)
	}
	return &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     new(big.Int).SetBytes(xBytes),
		Y:     new(big.Int).SetBytes(yBytes),
	}, nil
}

type VerificationMethod struct {
	ID           string       `json:"id"`
	PublicKeyJWK PublicKeyJWK `json:"publicKeyJwk"`
}

// ECPublicKey is the method's P-256 key.
func (m VerificationMethod) ECPublicKey() (*ecdsa.PublicKey, error) {
	return m.PublicKeyJWK.ECPublicKey()
}

// Purpose is a DID verification relationship (DID Core §5.3): what a document
// publishes a key as usable FOR. Every key here is resolved by the purpose the
// consumer needs and, where the consumer names one, by that name — never by its
// position in verificationMethod, which DID Core gives no meaning at all, and
// never by a key label that happens to be this deployment's.
type Purpose string

const (
	// PurposeAuthentication publishes a key as one the subject proves control of
	// to authenticate itself — the DCS-to-DCS challenge-response.
	PurposeAuthentication Purpose = "authentication"
	// PurposeAssertion publishes a key as one that may make assertions — a
	// credential proof, a JAdES over a contract.
	PurposeAssertion Purpose = "assertionMethod"
	// PurposeKeyAgreement publishes a key as one others may wrap keys to.
	PurposeKeyAgreement Purpose = "keyAgreement"
)

type DIDDocument struct {
	VerificationMethod []VerificationMethod `json:"verificationMethod"`
	// The verification relationships. An entry is either a reference to a
	// method's id — absolute, or relative to the document as "#key-1" — or the
	// method embedded inline; all three shapes are read (see methodsFor).
	Authentication  []any `json:"authentication"`
	AssertionMethod []any `json:"assertionMethod"`
	KeyAgreement    []any `json:"keyAgreement"`

	didContent map[string]interface{}
	signer     crypto.Signer
	// The method the bound signer's key is published as, and that key.
	signingMethod *VerificationMethod
	publicKey     *ecdsa.PublicKey
}

// ResolveMethodID returns the absolute form of a verification method id as used
// within docID. A bare fragment is relative to the document that carries it,
// which DID Core explicitly permits both in a relationship and in a proof, and
// an id naming a different document is refused: a key published elsewhere
// authorizes nothing here.
func ResolveMethodID(docID, methodID string) (string, error) {
	id := strings.TrimSpace(methodID)
	if id == "" {
		return "", errors.New("no verification method named")
	}
	if strings.HasPrefix(id, "#") {
		id = docID + id
	}
	if base, _, _ := strings.Cut(id, "#"); base != docID {
		return "", fmt.Errorf("verification method %q does not belong to %q", methodID, docID)
	}
	return id, nil
}

// methodsFor returns the verification methods the document publishes for a
// purpose. A relationship entry either references a method by id or embeds the
// method itself; an embedded entry that carries key material IS the method,
// since a document may publish a key in a relationship alone.
func (d *DIDDocument) methodsFor(purpose Purpose) ([]VerificationMethod, error) {
	docID, err := d.GetID()
	if err != nil {
		return nil, err
	}

	var entries []any
	switch purpose {
	case PurposeAuthentication:
		entries = d.Authentication
	case PurposeAssertion:
		entries = d.AssertionMethod
	case PurposeKeyAgreement:
		entries = d.KeyAgreement
	default:
		return nil, fmt.Errorf("unknown verification relationship %q", purpose)
	}

	methods := make([]VerificationMethod, 0, len(entries))
	for _, entry := range entries {
		var embedded VerificationMethod
		switch value := entry.(type) {
		case string:
			embedded.ID = value
		case map[string]any:
			raw, err := json.Marshal(value)
			if err != nil {
				continue
			}
			if err := json.Unmarshal(raw, &embedded); err != nil {
				continue
			}
		default:
			continue
		}

		id, err := ResolveMethodID(docID, embedded.ID)
		if err != nil {
			continue
		}
		embedded.ID = id
		if embedded.PublicKeyJWK.X != "" {
			methods = append(methods, embedded)
			continue
		}
		for i := range d.VerificationMethod {
			published, err := ResolveMethodID(docID, d.VerificationMethod[i].ID)
			if err != nil || published != id {
				continue
			}
			method := d.VerificationMethod[i]
			method.ID = published
			methods = append(methods, method)
			break
		}
	}
	return methods, nil
}

// MethodFor resolves the method a consumer NAMED — a proof's verificationMethod,
// a wrapped CEK's kid — and requires the document to publish it for the purpose
// the consumer needs it for. A named key the document publishes for something
// else is refused, not accepted: the relationships exist to keep the ECDH key
// from verifying signatures and the signing key from receiving wrapped keys.
func (d *DIDDocument) MethodFor(purpose Purpose, methodID string) (*VerificationMethod, error) {
	docID, err := d.GetID()
	if err != nil {
		return nil, err
	}
	id, err := ResolveMethodID(docID, methodID)
	if err != nil {
		return nil, err
	}
	methods, err := d.methodsFor(purpose)
	if err != nil {
		return nil, err
	}
	for i := range methods {
		if methods[i].ID == id {
			return &methods[i], nil
		}
	}
	return nil, fmt.Errorf("did document %s does not publish %q for %s", docID, id, purpose)
}

// AssertionKey returns the key a proof NAMES, provided the document publishes it
// as one that may make assertions.
//
// The verification method is taken from the proof rather than guessed. This
// instance labels its credential key `#dcs-vc`, but that is a local convention,
// not something DID Core requires: another implementation publishes `#key-1`, a
// UUID, or an absolute DID URL, and deriving the id from our own label works only
// for as long as every peer runs this software. A proof already says which key
// made it; the document says whether that key was allowed to.
//
// Being listed in assertionMethod is the authorization. A DID document
// deliberately separates its relationships — our own gendid publishes a
// key-agreement key in the same document — so a key that is merely present is not
// a key entitled to assert anything.
func (d *DIDDocument) AssertionKey(verificationMethodID string) (*ecdsa.PublicKey, error) {
	if d == nil {
		return nil, errors.New("no did document to resolve the proof's verification method in")
	}
	if strings.TrimSpace(verificationMethodID) == "" {
		return nil, errors.New("the proof names no verification method")
	}
	method, err := d.MethodFor(PurposeAssertion, verificationMethodID)
	if err != nil {
		return nil, err
	}
	return method.ECPublicKey()
}

// PublishesKeyFor reports whether the document publishes this exact public key
// for the purpose — the check for a consumer that carries a key but names no id,
// such as a JAdES whose x5c leaf has to be a key the peer may assert with.
func (d *DIDDocument) PublishesKeyFor(purpose Purpose, pub *ecdsa.PublicKey) bool {
	if pub == nil {
		return false
	}
	methods, err := d.methodsFor(purpose)
	if err != nil {
		return false
	}
	for i := range methods {
		published, err := methods[i].ECPublicKey()
		if err != nil {
			continue
		}
		if published.X.Cmp(pub.X) == 0 && published.Y.Cmp(pub.Y) == 0 {
			return true
		}
	}
	return false
}

// NewDIDDocument loads a DID document from disk and binds it to the given HSM
// signer, verifying that the signer's public key matches the DID document's
// published ECDSA P-256 verification method and validating the pairing with a
// test signature.
func NewDIDDocument(didFilePath string, signer crypto.Signer) (*DIDDocument, error) {
	didJSON, err := os.ReadFile(didFilePath)
	if err != nil {
		return nil, err
	}

	// Unmarshal into the struct -> fills VerificationMethod.
	var doc DIDDocument
	if err := json.Unmarshal(didJSON, &doc); err != nil {
		return nil, fmt.Errorf("decoding did.json: %w", err)
	}

	// Keep the raw content as a map alongside.
	if err := json.Unmarshal(didJSON, &doc.didContent); err != nil {
		return nil, fmt.Errorf("decoding did.json content: %w", err)
	}

	if len(doc.VerificationMethod) == 0 {
		return nil, errors.New("no verification methods in DID document")
	}

	if signer == nil {
		return nil, errors.New("did signer is required")
	}

	signerPub, ok := signer.Public().(*ecdsa.PublicKey)
	if !ok {
		return nil, errors.New("did signer public key is not ECDSA")
	}
	signingMethod, err := doc.methodHoldingKey(signerPub)
	if err != nil {
		return nil, err
	}
	pubKey, err := signingMethod.ECPublicKey()
	if err != nil {
		return nil, fmt.Errorf("extracting public key from DID document: %w", err)
	}

	// Self test: signing and verifying must work.
	message := []byte("key pair self test")
	hash := sha256.Sum256(message)

	signature, err := signer.Sign(rand.Reader, hash[:], crypto.SHA256)
	if err != nil {
		return nil, fmt.Errorf("key pair self test (sign): %w", err)
	}
	if !ecdsa.VerifyASN1(pubKey, hash[:], signature) {
		return nil, errors.New("key pair self test (verify) failed")
	}

	doc.signer = signer
	doc.signingMethod = signingMethod
	doc.publicKey = pubKey

	return &doc, nil
}

// methodHoldingKey finds the verification method that publishes the key this
// instance holds. The document states which key that is, so the pairing is found
// by matching the key rather than by taking the first method: the served
// document carries several (identity, credential, key agreement) and their order
// means nothing.
//
// It must be a key published for signing — authenticating this instance to a
// peer, or asserting — so a document that publishes the held key for key
// agreement alone cannot end up signing challenges and contracts with it.
func (d *DIDDocument) methodHoldingKey(pub *ecdsa.PublicKey) (*VerificationMethod, error) {
	for _, purpose := range []Purpose{PurposeAuthentication, PurposeAssertion} {
		methods, err := d.methodsFor(purpose)
		if err != nil {
			return nil, err
		}
		for i := range methods {
			published, err := methods[i].ECPublicKey()
			if err != nil {
				continue
			}
			if published.X.Cmp(pub.X) == 0 && published.Y.Cmp(pub.Y) == 0 {
				return &methods[i], nil
			}
		}
	}
	return nil, fmt.Errorf("did document publishes no %s or %s verification method holding the signer's public key",
		PurposeAuthentication, PurposeAssertion)
}

// PublicKey is the key the bound signer holds, as the document publishes it.
// A document merely resolved from a peer has no signer and no such key: what a
// peer's key is good for is decided per purpose (MethodFor, PublishesKeyFor).
func (d *DIDDocument) PublicKey() *ecdsa.PublicKey {
	return d.publicKey
}

// SigningMethod is the verification method this instance's signer is published
// as — the id a consumer names it by (a JAR's kid) and the certificate chain
// that backs it.
func (d *DIDDocument) SigningMethod() *VerificationMethod {
	return d.signingMethod
}

func (d DIDDocument) GetDIDContent() map[string]interface{} {
	return d.didContent
}

func (d DIDDocument) GetID() (string, error) {
	raw, ok := d.didContent["id"]
	if !ok {
		return "", errors.New(`did document does not contain "id"`)
	}

	id, ok := raw.(string)
	if !ok {
		return "", errors.New(`did document "id" is not a string`)
	}

	return id, nil
}

func (d DIDDocument) GetHostname() (string, error) {
	id, err := d.GetID()
	if err != nil {
		return "", err
	}
	return DIDWebToHostname(id)
}

// OwnKeyAgreementMethod returns the keyAgreement method publishing the key this
// instance's HSM holds under the given CKA_LABEL. Resolving OUR OWN key by our
// own label is knowledge we have; the fragment convention only ever describes
// this deployment's document, which is why nothing resolves a PEER's key this
// way.
func (d *DIDDocument) OwnKeyAgreementMethod(label string) (*VerificationMethod, error) {
	methods, err := d.methodsFor(PurposeKeyAgreement)
	if err != nil {
		return nil, err
	}
	suffix := "#" + label
	for i := range methods {
		if strings.HasSuffix(methods[i].ID, suffix) {
			return &methods[i], nil
		}
	}
	return nil, fmt.Errorf("did document publishes no %s method for the token key %q", PurposeKeyAgreement, label)
}

// PeerKeyAgreementMethod returns the method a content-encryption key is to be
// wrapped to for this document's subject, and which the wrap then NAMES in its
// kid so the recipient resolves the key instead of the sender assuming it holds
// exactly one.
//
// Several are legitimate — a rotation publishes the new key beside the old — and
// DID Core defines no precedence, so the subject's own first entry is taken: the
// order of the relationship is the subject's statement about its own keys, not
// an assumption about the layout of verificationMethod.
func (d *DIDDocument) PeerKeyAgreementMethod() (*VerificationMethod, error) {
	methods, err := d.methodsFor(PurposeKeyAgreement)
	if err != nil {
		return nil, err
	}
	if len(methods) == 0 {
		return nil, fmt.Errorf("did document publishes no %s method to wrap a key to", PurposeKeyAgreement)
	}
	return &methods[0], nil
}

// Sign signs content with ECDSA (SHA-256), returning an ASN.1 DER signature.
func (d *DIDDocument) Sign(content []byte) ([]byte, error) {
	if d.signer == nil {
		return nil, errors.New("signer not set")
	}

	hash := sha256.Sum256(content)
	return d.signer.Sign(rand.Reader, hash[:], crypto.SHA256)
}

// VerifyPeerChallenge runs layers 1 and 2 of the peer trust model against ONE
// key: the challenge-response must be answered by a key this document publishes
// for authenticating its subject, and the certificate chain then validated is
// that same key's. Verifying a signature against one key and a chain belonging
// to another says nothing about either.
//
// The response names no key, so every key published for the purpose is a
// candidate and only those are — a key published for key agreement can never
// authenticate the peer. Both authentication and assertionMethod publish a key
// as one the subject controls and speaks with, and gendid publishes this
// instance's identity key in both, so either relationship carries a peer.
func (d *DIDDocument) VerifyPeerChallenge(trustPool *EUTrustPool, content, signature []byte) error {
	hash := sha256.Sum256(content)

	var candidates []VerificationMethod
	for _, purpose := range []Purpose{PurposeAuthentication, PurposeAssertion} {
		methods, err := d.methodsFor(purpose)
		if err != nil {
			return err
		}
		candidates = append(candidates, methods...)
	}
	if len(candidates) == 0 {
		return fmt.Errorf("did document publishes no %s or %s method, so nothing in it may authenticate its subject",
			PurposeAuthentication, PurposeAssertion)
	}

	for i := range candidates {
		pub, err := candidates[i].ECPublicKey()
		if err != nil {
			continue
		}
		if ecdsa.VerifyASN1(pub, hash[:], signature) {
			return d.verifyCertificateOf(&candidates[i], trustPool)
		}
	}
	return errors.New("challenge response verifies against none of the keys the did document publishes for authenticating its subject")
}

// verifyCertificateOf validates the x5c certificate chain of one verification
// method:
//
//  1. Chain validation leaf -> intermediates -> root against trustPool.
//  2. The leaf certificate must match the hostname of the DID.
//  3. The public key of the leaf must match the JWK (x/y).
//  4. The leaf must carry the eIDAS QcCompliance statement.
//
// Steps 1 and 4 are performed only if trustPool holds a populated pool; a nil
// or unrefreshed pool reduces the check to steps 2 and 3. QCStatements are a
// self-declaration by the issuer, so a legally binding eIDAS validation
// additionally requires the pool to be built from the EU Trusted Lists
// (LOTL/TSL) via EUTrustPool.Refresh.
func (d *DIDDocument) verifyCertificateOf(method *VerificationMethod, trustPool *EUTrustPool) error {
	if method == nil {
		return errors.New("no verification method to validate a certificate chain for")
	}

	var trustedRoots *x509.CertPool
	if trustPool != nil {
		trustedRoots = trustPool.Pool()
	}

	certs, err := certificateChain(method)
	if err != nil {
		return fmt.Errorf("verification method %q: %w", method.ID, err)
	}

	if trustedRoots != nil {
		if err := verifyChain(certs, trustedRoots); err != nil {
			return err
		}
	}

	cert := certs[0]

	// 1. Does the certificate match the hostname of the DID?
	hostname, err := d.GetHostname()
	if err != nil {
		return err
	}
	host := hostname
	if h, _, err := net.SplitHostPort(hostname); err == nil {
		host = h // strip port, e.g. "localhost:8991" -> "localhost"
	}
	if err := cert.VerifyHostname(host); err != nil {
		return fmt.Errorf("certificate does not match hostname %q: %w", host, err)
	}

	// 2. Does the certificate match the public key from the JWK?
	certPub, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return errors.New("certificate does not contain an ECDSA public key")
	}
	jwkPub, err := method.ECPublicKey()
	if err != nil {
		return err
	}
	if certPub.X.Cmp(jwkPub.X) != 0 || certPub.Y.Cmp(jwkPub.Y) != 0 {
		return errors.New("certificate public key does not match JWK public key")
	}

	if trustedRoots != nil {
		// 3. Does the certificate carry the eIDAS QCStatements?
		qualified, qscd, err := parseQCStatements(cert)
		if err != nil {
			return err
		}
		if !qualified {
			return errors.New("certificate is not an eIDAS qualified certificate (QcCompliance statement missing)")
		}
		_ = qscd // optional: enforce additionally if required
	}

	return nil
}

// VerifyEIDASCertificate validates the chain of the key THIS instance signs
// with, as the document publishes it (SigningMethod). A peer's document is
// checked against the key that actually answered its challenge instead —
// VerifyPeerChallenge — since nothing else in a resolved document is known to be
// the peer's identity key.
func (d *DIDDocument) VerifyEIDASCertificate(trustPool *EUTrustPool) error {
	if d.signingMethod == nil {
		return errors.New("did document is not bound to a signer, so it has no own certificate chain to validate")
	}
	return d.verifyCertificateOf(d.signingMethod, trustPool)
}

// certificateChain parses all x5c entries of one verification method. Entries
// starting with http:// or https:// are fetched remotely (PEM or DER), all
// others are interpreted as base64 DER (standard base64 per RFC 7517, NOT
// base64url).
func certificateChain(method *VerificationMethod) ([]*x509.Certificate, error) {
	x5c := method.PublicKeyJWK.X5C
	if len(x5c) == 0 {
		return nil, errors.New("no x5c entry in publicKeyJwk")
	}

	certs := make([]*x509.Certificate, 0, len(x5c))
	for i, entry := range x5c {
		var der []byte
		var err error

		if strings.HasPrefix(entry, "http://") || strings.HasPrefix(entry, "https://") {
			der, err = fetchCertificateDER(entry)
		} else {
			der, err = base64.StdEncoding.DecodeString(entry)
		}
		if err != nil {
			return nil, fmt.Errorf("x5c[%d]: %w", i, err)
		}

		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return nil, fmt.Errorf("parsing x5c[%d]: %w", i, err)
		}
		certs = append(certs, cert)
	}
	return certs, nil
}

// fetchTimeout bounds every outbound fetch this file makes (a peer's
// did.json, or an x5c chain entry fetched by URL): http.DefaultClient has no
// timeout, so a peer that accepts the connection and never responds would
// otherwise hang the caller (PostPdf's inbound verification, the outbound
// trust gate's did.json fetch) indefinitely instead of failing.
var fetchTimeout = 10 * time.Second

// The two clients every outbound fetch here uses, differing only in whether
// loopback may be dialled — see fetchClientForURL.
var (
	fetchClientStrict   = safehttp.Client(fetchTimeout, safehttp.Policy{})
	fetchClientLoopback = safehttp.Client(fetchTimeout, safehttp.Policy{AllowLoopback: true})
)

// fetchClientForURL picks the client for one outbound fetch: no redirects, and
// no dialling an address a published identity never lives on (safehttp).
//
// Loopback is permitted exactly when DIDWebSchemes has already decided this
// host is a loopback one — the rule that lets the dev and CI stacks resolve
// each other over http://*.localhost — so loopback is decided once here rather
// than by a second rule that can drift away from the first.
//
// The redirect refusal matters most: following one let the responder choose the
// next address after the first had been checked, and an https -> http hop
// silently undid the https-only rule DIDWebSchemes exists to enforce.
func fetchClientForURL(rawURL string) (*http.Client, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", rawURL, err)
	}
	if isLoopbackHost(parsed.Host) || isInsecureDIDWebHost(parsed.Host) {
		return fetchClientLoopback, nil
	}
	return fetchClientStrict, nil
}

// fetchCertificateDER fetches a certificate from a URL and returns it as
// DER. The server may deliver PEM or raw DER.
func fetchCertificateDER(certURL string) ([]byte, error) {
	client, err := fetchClientForURL(certURL)
	if err != nil {
		return nil, err
	}
	resp, err := client.Get(certURL)
	if err != nil {
		return nil, fmt.Errorf("fetching certificate from %s: %w", certURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %s fetching certificate from %s", resp.Status, certURL)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading certificate: %w", err)
	}

	if block, _ := pem.Decode(body); block != nil {
		return block.Bytes, nil
	}
	return body, nil
}

// verifyChain validates the signature chain leaf -> intermediates -> root.
//
// trustedRoots determines the trust anchor:
//   - nil: system trust store of the operating system.
//   - custom pool: e.g. populated from the EU Trusted Lists.
//
// Self-signed certificates from the supplied chain are deliberately NOT
// accepted as trust anchors — otherwise an attacker could ship their own
// root and the check would be worthless.
func verifyChain(certs []*x509.Certificate, trustedRoots *x509.CertPool) error {
	if len(certs) == 0 {
		return errors.New("empty certificate chain")
	}
	leaf := certs[0]

	intermediates := x509.NewCertPool()
	for _, c := range certs[1:] {
		intermediates.AddCert(c)
	}

	_, err := leaf.Verify(x509.VerifyOptions{
		Intermediates: intermediates,
		Roots:         trustedRoots, // nil -> system roots
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	})
	if err != nil {
		return fmt.Errorf("certificate chain verification failed: %w", err)
	}
	return nil
}

// parseQCStatements looks for the QCStatements extension and reports
// whether QcCompliance (qualified certificate) and QcSSCD (QSCD) are set.
func parseQCStatements(cert *x509.Certificate) (qualified bool, qscd bool, err error) {
	for _, ext := range cert.Extensions {
		if !ext.Id.Equal(oidQCStatements) {
			continue
		}
		var statements []qcStatement
		if _, err := asn1.Unmarshal(ext.Value, &statements); err != nil {
			return false, false, fmt.Errorf("parsing QCStatements: %w", err)
		}
		for _, s := range statements {
			switch {
			case s.StatementID.Equal(oidQcCompliance):
				qualified = true
			case s.StatementID.Equal(oidQcSSCD):
				qscd = true
			}
		}
		return qualified, qscd, nil
	}
	return false, false, nil // extension not present -> not an eIDAS certificate
}

// FetchDIDDocument resolves a did:web identifier to its document over https —
// and over http only for the hosts DIDWebSchemes permits it for. Resolution
// follows the identifier's own path segments, so several instances can share
// one host.
func FetchDIDDocument(did string) (*DIDDocument, error) {
	host, segments, err := DIDWebPath(did)
	if err != nil {
		return nil, err
	}
	docPath := DIDWebDocumentPath(segments)

	var lastErr error
	for _, scheme := range DIDWebSchemes(host) {
		doc, err := fetchDIDDocumentFromURL(DIDWebBaseURL(scheme, host, nil) + docPath)
		if err == nil {
			return doc, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("fetching did.json for %s failed: %w", did, lastErr)
}

// DIDWebToHostname extracts the host (including port) from a did:web
// identifier, e.g. "did:web:localhost%3A8991" -> "localhost:8991".
//
// Path segments are deliberately dropped: this is the authority, which is what
// certificate hostname verification checks. Use DIDWebPath when the whole
// resolution target matters — two DIDs under one host share a hostname.
func DIDWebToHostname(did string) (string, error) {
	host, _, err := DIDWebPath(did)
	return host, err
}

// DIDWebPath splits a did:web identifier into its authority and its path
// segments, e.g. "did:web:example.com%3A8991:tenant:b" ->
// ("example.com:8991", ["tenant", "b"]).
//
// The authority is decoded strictly: %3A, the port separator, is the only
// escape did:web defines there, and everything else is refused rather than
// decoded. Decoding arbitrary escapes turned "did:web:evil.example%2F..%2F.."
// into the authority "evil.example/../..", which DIDWebBaseURL concatenates
// straight into a URL — the identifier then picks the path that every
// resolution, agreement-credential fetch and peer request lands on, and a
// host check earlier in the flow has been left behind. A separator sitting in
// the authority literally does the same without any escape at all, so the
// decoded authority has to look like a host, not merely decode to one.
func DIDWebPath(did string) (string, []string, error) {
	const prefix = "did:web:"
	if !strings.HasPrefix(did, prefix) {
		return "", nil, fmt.Errorf("not a did:web identifier: %q", did)
	}

	parts := strings.Split(strings.TrimPrefix(did, prefix), ":")
	host, err := didWebAuthority(parts[0])
	if err != nil {
		return "", nil, fmt.Errorf("did:web identifier %q: %w", did, err)
	}

	segments := make([]string, 0, len(parts)-1)
	for _, raw := range parts[1:] {
		segment, err := url.PathUnescape(raw)
		if err != nil {
			return "", nil, fmt.Errorf("invalid percent-encoding in did:web path: %w", err)
		}
		if segment == "" {
			return "", nil, fmt.Errorf("did:web identifier %q has an empty path segment", did)
		}
		// A segment that decodes to a separator or a traversal is a path the
		// identifier writes rather than a name it carries.
		if strings.ContainsAny(segment, `/\?#`) || segment == "." || segment == ".." {
			return "", nil, fmt.Errorf("did:web identifier %q has a path segment %q that would rewrite its own document path", did, segment)
		}
		segments = append(segments, segment)
	}
	return host, segments, nil
}

// didWebHost is the shape the decoded authority must have: a host name, and at
// most a numeric port. Deliberately no percent signs, separators, credentials
// or bracketed IPv6 literal — none of which any deployment publishes, and each
// of which changes where the document is read from.
var didWebHost = regexp.MustCompile(`^[A-Za-z0-9._-]+(:[0-9]{1,5})?$`)

func didWebAuthority(raw string) (string, error) {
	if raw == "" {
		return "", errors.New("has empty host component")
	}

	var decoded strings.Builder
	for i := 0; i < len(raw); i++ {
		if raw[i] != '%' {
			decoded.WriteByte(raw[i])
			continue
		}
		if i+3 > len(raw) || !strings.EqualFold(raw[i:i+3], "%3a") {
			return "", fmt.Errorf("host component %q carries a percent-escape other than %%3A; only the port separator is encoded in a did:web authority", raw)
		}
		decoded.WriteByte(':')
		i += 2
	}

	// DNS names are case-insensitive, so two identifiers differing only in the
	// case of the authority name one and the same host. Normalising here makes
	// everything derived from it — the resolution URL, the agreement-credential
	// URL, the issuer comparison in the trust gate — agree on that, instead of
	// comparing the two spellings as strings and concluding they are different
	// peers. Path segments are NOT normalised: those are case-sensitive.
	host := strings.ToLower(decoded.String())
	if !didWebHost.MatchString(host) {
		return "", fmt.Errorf("host component %q is not a hostname with an optional port", host)
	}
	return host, nil
}

// DIDWebDocumentPath is the path a did:web document is served at, per
// did-method-web: the bare authority uses /.well-known/did.json, while an
// identifier carrying path segments uses those segments and NO .well-known.
// Getting this wrong makes every DID under one host resolve to the same
// document.
func DIDWebDocumentPath(segments []string) string {
	if len(segments) == 0 {
		return "/.well-known/did.json"
	}
	return "/" + strings.Join(segments, "/") + "/did.json"
}

// DIDWebBaseURL is the origin plus path segments a did:web identifier denotes,
// without the document filename — the base an instance's own endpoints hang off.
func DIDWebBaseURL(scheme, host string, segments []string) string {
	base := scheme + "://" + host
	if len(segments) > 0 {
		base += "/" + strings.Join(segments, "/")
	}
	return base
}

func fetchDIDDocumentFromURL(docURL string) (*DIDDocument, error) {
	client, err := fetchClientForURL(docURL)
	if err != nil {
		return nil, err
	}
	resp, err := client.Get(docURL)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %s from %s", resp.Status, docURL)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading did.json: %w", err)
	}

	var doc DIDDocument
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("decoding did.json: %w", err)
	}

	if err := json.Unmarshal(body, &doc.didContent); err != nil {
		return nil, fmt.Errorf("decoding did.json content: %w", err)
	}

	if len(doc.VerificationMethod) == 0 {
		return nil, fmt.Errorf("no verification methods in DID document")
	}

	return &doc, nil
}

// DIDWebSchemes returns the URL schemes a did:web document may be fetched over.
//
// The method mandates HTTPS. Falling back to plaintext let an on-path attacker
// serve both the DID document holding a peer's key AND the agreement credential
// verified against it, which collapses the federation gate entirely. Loopback
// is the exception, because the dev and CI stacks resolve each other over
// http://*.localhost and no attacker sits on that path.
func DIDWebSchemes(host string) []string {
	if isLoopbackHost(host) || isInsecureDIDWebHost(host) {
		return []string{"https", "http"}
	}
	return []string{"https"}
}

// isInsecureDIDWebHost reports whether a deployment has named this host as one
// did:web may be resolved from over plain http.
//
// did:web is https, and loopback is the only exception the code makes on its
// own. But a cluster-internal identity is published by a Service on plain http
// under a name that is not loopback — the BDD stack resolves
// did:web:dcs-orce%3A1880 that way — and an https-only rule turns that into a
// resolution failure reported as an unrelated 500. Naming those hosts in
// DCS_DIDWEB_INSECURE_HOSTS keeps the exception explicit and per-deployment,
// rather than reinstating a silent fallback that also applies on the internet.
func isInsecureDIDWebHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}
	for _, allowed := range strings.Split(os.Getenv("DCS_DIDWEB_INSECURE_HOSTS"), ",") {
		if allowed = strings.ToLower(strings.TrimSpace(allowed)); allowed != "" && allowed == host {
			return true
		}
	}
	return false
}

func isLoopbackHost(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	if idx := strings.LastIndex(h, ":"); idx != -1 {
		if _, err := strconv.Atoi(h[idx+1:]); err == nil {
			h = h[:idx]
		}
	}
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return true
	}
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsLoopback()
	}
	return false
}
