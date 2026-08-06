package oid4vp

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"strings"

	"digital-contracting-service/internal/auth/oid4vp/sdjwt"
)

// TrustConfig is the verifier trust anchor loaded from trust.dev.json (OID4VP_TRUST_DATA_PATH).
// It records which credential types and issuer DIDs are accepted, and bundles their JWKS.
// JWT public-key resolution for issuer signatures is in sdjwt/keys.go.
type TrustConfig struct {
	VCTs    []string                 `json:"vcts"`
	Issuers map[string]TrustedIssuer `json:"issuers"`

	// x5cRoots are the two CA trust lists an x5c-bearing credential's chain is
	// verified against (ADR-35): one for parties — peer credentials, and the
	// login issuers that are the same certificates seen from this side — and one
	// for PID. A real EUDI-wallet-issued PID carries its issuer certificate this
	// way, not a bare JWK. The dev and BDD stacks exercise that path too,
	// against the committed dev CA (backend/config/oid4vp/x5c-trust-anchors.dev.pem,
	// which LoadX5CTrustAnchors accepts only under DCS_ALLOW_DEV_TRUST).
	//
	// One flat pool meant a certificate under ANY configured anchor could sign
	// for any x5c issuer, held apart only by the leaf naming that issuer and by
	// the entry's purposes. With peers no longer enumerated the second of those
	// disappears for `peer`, so the list has to be the boundary it was being
	// credited as — and a PID CA must not be one of the CAs that can authorize a
	// signature. A set with no anchors refuses every x5c credential for it,
	// which is the correct failure: an x5c header proves nothing about whose leaf
	// it is without an anchor to verify against.
	//
	// Keyed by ANCHOR SET, of which there are two — see anchorSetFor. Held as
	// certificates rather than finished pools because a set accumulates anchors
	// from more than one source (a mounted bundle, plus this deployment's own
	// provisioned root), and a CertPool cannot be enumerated once built, so a
	// second contribution could only replace the first.
	x5cRoots map[Purpose][]*x509.Certificate

	// ORCEResolverURL is the flow endpoint the orce mechanism delegates to
	// (OID4VP_ORCE_RESOLVER_URL). Empty unless a deployment uses it.
	ORCEResolverURL string `json:"orce_resolver_url"`

	keyFetcher KeyFetcher
}

// Purpose is what an issuer's credentials may be used FOR. Trusting an issuer's
// signature says a credential is authentic; it does not say its holder may act
// here. Keeping the two apart is what lets an instance verify a counterparty's
// Power of Attorney without also accepting it as a login (ADR-31).
type Purpose string

const (
	// PurposeLogin: credentials from this issuer may act HERE — grant a session,
	// and authorize a signature at this instance's own ceremony. Both are the
	// same question ("may this holder act as a party on this deployment"), and
	// both are answered by an issuer this operator named and pinned.
	PurposeLogin Purpose = "login"
	// PurposePeer: credentials issued by ANOTHER DCS INSTANCE's issuer — the
	// Power of Attorney behind a signature applied over there, embedded into the
	// contract PDF as an associated file BEFORE that instance's own signature,
	// so its signature covers its own authorization and the PDF carries
	// everything the receiver needs (ADR-13: the PDF is the wire format).
	// Verified here on receipt (VerifyCounterpartyPoA,
	// dcstodcs.CounterpartyPoAGate).
	//
	// It never authorizes anything here. That is what lets it be admitted by a
	// chain to the PoA CA list rather than by an entry: we cannot enumerate who
	// a counterparty's issuer is, and we do not have to, because believing their
	// attestation is not the same as letting it act.
	PurposePeer Purpose = "peer"
	// PurposePID: credentials from this issuer attest the identity of a natural
	// person. A PID is a third party's attestation — an instance that issued it
	// to itself has attested nothing.
	PurposePID Purpose = "pid"
)

func knownPurpose(p Purpose) bool {
	switch p {
	case PurposeLogin, PurposePeer, PurposePID:
		return true
	}
	return false
}

// Mechanism names how an issuer's verification key is resolved. Deployments
// differ in how issuers publish keys, and the production model is not yet
// settled, so this is configuration rather than a compiled-in assumption.
type Mechanism string

const (
	MechanismJWKS   Mechanism = "jwks"    // keys bundled in the trust entry
	MechanismX5C    Mechanism = "x5c"     // chain in the credential header, verified to configured roots
	MechanismDIDJWK Mechanism = "did:jwk" // key decoded from the issuer identifier
	MechanismDIDWeb Mechanism = "did:web" // key fetched from the issuer's DID document
	MechanismORCE   Mechanism = "orce"    // delegated to a configured ORCE flow
)

// supportedMechanisms are the ones this build can resolve. A deployment naming
// anything else is refused AT LOAD rather than at first use, so an unsupported
// trust configuration surfaces on startup instead of when a wallet arrives.
//
// A scheme absent from this list — did:ebsi, a national registry, whatever
// comes next — is reached through MechanismORCE without a change here: the flow
// resolves it and answers with a JWKS.
var supportedMechanisms = map[Mechanism]bool{
	MechanismJWKS:   true,
	MechanismX5C:    true,
	MechanismDIDJWK: true,
	MechanismDIDWeb: true,
	MechanismORCE:   true,
}

// TrustedIssuer is one issuer entry: what it may be trusted for, which
// organizations it may speak for, and how its key is resolved.
type TrustedIssuer struct {
	Purposes []Purpose `json:"purposes"`
	// Organizations bounds what this issuer may attest. A credential naming an
	// organization absent from this list is refused even when the signature is
	// good — otherwise any trusted issuer could speak for any party, and an
	// organization check at the callsite would depend on every issuer being
	// well-behaved rather than on something the verifier enforces.
	// Not required for a pid issuer: a PID attests a person, not a party.
	Organizations []string        `json:"organizations"`
	Mechanism     Mechanism       `json:"mechanism"`
	JWKS          json.RawMessage `json:"jwks"`
	// X5CLeafKeys pins the public keys a LOGIN issuer's x5c leaf may carry,
	// base64 DER SubjectPublicKeyInfo (ADR-35). It is not a second way to
	// publish a key — the credential still carries its chain, and the mechanism
	// is still x5c. It states which leaf under that chain this deployment's
	// operator actually named, so no certificate authority can introduce
	// another login issuer here.
	//
	// A key rather than a whole certificate: the issuer re-mints its leaf when
	// its public URL changes and keeps the key, so pinning the certificate
	// would refuse a legitimate reissue. DER rather than a JWK so it covers
	// every key type a real issuer might hold, not only EC.
	X5CLeafKeys []string `json:"x5c_leaf_keys"`
}

// Allows reports whether this entry lists the purpose.
//
// Load-time schema validation only — it answers "is this document well-formed",
// not "is this issuer authorized", which is policy/trust.rego's question. Do not
// reach for it to make a trust decision: a second copy of that rule is a second
// thing to keep in step with the policy.
func (t TrustedIssuer) Allows(p Purpose) bool {
	for _, granted := range t.Purposes {
		if granted == p {
			return true
		}
	}
	return false
}

// OrganizationsAny is the explicit wildcard: this issuer is authoritative for
// whichever organization it names. It suits an issuer that IS the tenant
// registry for its deployment — it knows its organizations, the verifier does
// not, and enumerating them in trust configuration would mean editing that file
// whenever a tenant is onboarded.
//
// It has to be written out. Treating an absent list as "any" is how an issuer
// silently gains the right to speak for a party nobody granted it.
const OrganizationsAny = "*"

// devIssuerKeySources are the x-coordinates of every private key committed to
// this repository, and the file each one lives in. Anyone with a clone holds
// the private half, so an issuer keyed to one can mint any credential this
// instance would accept.
//
// The dev fixture is baked into the runtime image and is the chart default, so
// nothing stopped a deployment from trusting it — the rule was written in two
// documents and enforced nowhere. Set DCS_ALLOW_DEV_TRUST=true for the dev and
// CI stacks, which is the one place it is legitimate.
var devIssuerKeySources = map[string]string{
	"sAYnZiIkBGJWkgViAZy4Jsdsp3DXnL1mV7hYQKJYKss": "testWallet/keys/issuer-dev.jwk",
	"rnizNORb2RpCt7obNCoi9-6IE6dM6cj2TLue-zwvTZc": "testWallet/keys/issuer-dev-x5c.jwk, which is also the CA key in backend/config/oid4vp/x5c-trust-anchors.dev.pem",
	"TPg_7qbilFLESVua3__W5v-5PiqqmJWvb5l4jrrXvS4": "testWallet/keys/wallet.jwk",
	// The root the dev/BDD ORCE credential issuer is handed instead of
	// generating one, so its status lists and credentials chain to an anchor
	// that exists before the issuer has ever booted (scripts/orce-dev-root-ca.py).
	"oZV6WnfYJyAtBAvwpIywxo_KTCHOOhRcHb7lC9fvEDU": "deployment/helm/charts/orce/pki-dev/root-ca.key, the CA the dev/BDD ORCE issuer signs under",
	// The issuer key itself, mounted so the dev/BDD stacks can PIN it as their
	// login issuer (ADR-35). A login issuer is verified against the key an
	// operator wrote down, and a key generated on the pod's volume at boot is
	// not knowable when that trust document is committed.
	"fOZixFX_H2zxj4vP0MFU1yRBJO9YQ2XV3sbokuw_rZw": "deployment/helm/charts/orce/pki-dev/issuer.key, the leaf key the dev/BDD ORCE issuer signs credentials with",
}

// devIssuerKeyX is devIssuerKeySources keyed by the canonical form of each
// coordinate, which is what a candidate key is compared against.
var devIssuerKeyX = func() map[string]string {
	byValue := make(map[string]string, len(devIssuerKeySources))
	for x, source := range devIssuerKeySources {
		byValue[canonicalCoordinate(x)] = source
	}
	return byValue
}()

// canonicalCoordinate reduces an EC coordinate to the number it denotes.
// Verification decodes coordinates into a big.Int (sdjwt.decodeCoordinate), so
// a leading-zero-padded encoding and the bare one are the same key; comparing
// the base64 text, as this guard used to, let a re-encoding of committed
// material walk straight past it.
func canonicalCoordinate(value string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(value), "=")
	raw, err := base64.RawURLEncoding.DecodeString(trimmed)
	if err != nil {
		raw, err = base64.RawStdEncoding.DecodeString(trimmed)
	}
	if err != nil {
		// Undecodable material verifies nothing either way, but comparing it
		// literally still recognises a mangled copy of a committed key.
		return "raw:" + trimmed
	}
	return hex.EncodeToString(bytes.TrimLeft(raw, "\x00"))
}

func canonicalCoordinateFromInt(x *big.Int) string {
	if x == nil {
		return ""
	}
	return hex.EncodeToString(x.Bytes())
}

// devKeyMaterial names the committed key an issuer entry is keyed to, or "".
//
// Every form in which an issuer can publish that key is inspected, not just a
// bundled JWKS: the same private key reaches the verifier equally well through
// a certificate in an x5c member, or through a did:jwk identifier that IS the
// key. A guard reading only `jwks` cannot see either, so an issuer had only to
// state the same material a different way.
//
// A did:web issuer is the one form that cannot be judged here — its keys live
// in a document fetched at verification time. Its authority is what it stakes,
// and this instance is not the one hosting it.
func devKeyMaterial(iss string, entry TrustedIssuer) string {
	if source := devKeyInJWKS(entry.JWKS); source != "" {
		return source
	}
	if source := devKeyInLeafPins(entry.X5CLeafKeys); source != "" {
		return source
	}
	if strings.HasPrefix(strings.TrimSpace(iss), "did:jwk:") {
		if key, err := sdjwt.JWKFromDIDJWK(strings.TrimSpace(iss)); err == nil {
			if source, ok := devIssuerKeyX[canonicalCoordinate(key.X)]; ok {
				return source
			}
		}
	}
	return ""
}

// devKeyInLeafPins names the committed key a login issuer's leaf pin holds.
//
// A pin is key material like any other: it says which key this deployment
// believes its own login issuer by, so pinning one whose private half ships in
// this repository lets anyone with a clone mint a session here. The guard read
// only `jwks`, which is exactly the form an x5c issuer does not use — so the
// one entry shape that carries a key for the purpose with no CA above it was
// the one shape nothing checked.
func devKeyInLeafPins(pins []string) string {
	for _, encoded := range pins {
		der, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
		if err != nil {
			continue
		}
		key, err := x509.ParsePKIXPublicKey(der)
		if err != nil {
			continue
		}
		pub, ok := key.(*ecdsa.PublicKey)
		if !ok {
			continue
		}
		if source, ok := devIssuerKeyX[canonicalCoordinateFromInt(pub.X)]; ok {
			return source
		}
	}
	return ""
}

func devKeyInJWKS(raw json.RawMessage) string {
	if len(bytes.TrimSpace(raw)) == 0 {
		return ""
	}

	type jwkProbe struct {
		X   string   `json:"x"`
		X5C []string `json:"x5c"`
	}
	var doc struct {
		Keys []jwkProbe `json:"keys"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return ""
	}
	if len(doc.Keys) == 0 {
		// A bare JWK where a JWKS was expected is still key material.
		var single jwkProbe
		if err := json.Unmarshal(raw, &single); err != nil {
			return ""
		}
		doc.Keys = []jwkProbe{single}
	}

	for _, key := range doc.Keys {
		if source, ok := devIssuerKeyX[canonicalCoordinate(key.X)]; ok {
			return source
		}
		for _, certB64 := range key.X5C {
			der, err := base64.StdEncoding.DecodeString(strings.TrimSpace(certB64))
			if err != nil {
				continue
			}
			cert, err := x509.ParseCertificate(der)
			if err != nil {
				continue
			}
			if source, ok := devCertificateKey(cert); ok {
				return source
			}
		}
	}
	return ""
}

// devCertificateKey names the committed key a certificate carries, if any.
// The dev CA is self-signed with testWallet/keys/issuer-dev-x5c.jwk, so a
// certificate — anchor or leaf — is recognised by its key rather than by a
// fingerprint that a regenerated fixture would invalidate.
func devCertificateKey(cert *x509.Certificate) (string, bool) {
	pub, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return "", false
	}
	source, ok := devIssuerKeyX[canonicalCoordinateFromInt(pub.X)]
	return source, ok
}

// devTrustAllowed reports whether this stack has said out loud that trusting
// repo-committed key material is legitimate here.
func devTrustAllowed() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("DCS_ALLOW_DEV_TRUST")), "true")
}

// LoadTrustConfig reads trust data from a JSON file (ConfigMap mount).
func LoadTrustConfig(path string) (*TrustConfig, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("trust config path is empty")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read trust config %q: %w", path, err)
	}

	var cfg TrustConfig
	err = json.Unmarshal(data, &cfg)

	if err != nil {
		return nil, fmt.Errorf("parse trust config %q: %w", path, err)
	}

	if len(cfg.VCTs) == 0 {
		return nil, fmt.Errorf("trust config %q: vcts is required", path)
	}

	if len(cfg.Issuers) == 0 {
		return nil, fmt.Errorf("trust config %q: issuers is required", path)
	}

	// Keys are canonicalised once, here, so every lookup afterwards — the
	// policy input, the mechanism resolver, the listed/unlisted split ADR-35
	// turns on — agrees on what this issuer is called. A did:web re-spelled in
	// another case would otherwise miss its own entry and be treated as
	// unlisted, which now means "try the anchored peer path" rather than
	// "refuse".
	canonical := make(map[string]TrustedIssuer, len(cfg.Issuers))
	for iss, entry := range cfg.Issuers {
		key := canonicalIssuerKey(iss)
		if existing, clash := canonical[key]; clash {
			_ = existing
			return nil, fmt.Errorf("trust config %q: issuers %q and %q are the same identifier written two ways; one entry has to be the operator's complete answer for it", path, iss, key)
		}
		canonical[key] = entry
	}
	cfg.Issuers = canonical

	allowDev := devTrustAllowed()

	for iss, entry := range cfg.Issuers {
		if !allowDev {
			if source := devKeyMaterial(iss, entry); source != "" {
				return nil, fmt.Errorf("trust config %q: issuer %q is keyed to material committed in this repository (%s), so anyone holding a clone could mint credentials it would accept; set DCS_ALLOW_DEV_TRUST=true if this really is a development stack", path, iss, source)
			}
		}
		if len(entry.Purposes) == 0 {
			return nil, fmt.Errorf("trust config %q: issuer %q declares no purposes; an entry without purposes would have to mean either none or all, and defaulting to all is how a peer's issuer silently becomes a login issuer", path, iss)
		}
		for _, p := range entry.Purposes {
			if !knownPurpose(p) {
				return nil, fmt.Errorf("trust config %q: issuer %q declares unknown purpose %q", path, iss, p)
			}
		}
		if entry.Mechanism == "" {
			return nil, fmt.Errorf("trust config %q: issuer %q declares no mechanism", path, iss)
		}
		if !supportedMechanisms[entry.Mechanism] {
			return nil, fmt.Errorf("trust config %q: issuer %q declares mechanism %q, which this build cannot resolve", path, iss, entry.Mechanism)
		}
		// The mechanism decides how a key is resolved, and an x5c issuer
		// resolves from its chain. Bundling a JWKS beside it states two
		// resolutions for one issuer, and only one of them can be what the
		// operator meant. The leaf pin below is NOT that: it constrains the
		// chain rather than offering an alternative to it.
		if entry.Mechanism == MechanismX5C && len(entry.JWKS) != 0 {
			return nil, fmt.Errorf("trust config %q: issuer %q resolves through a certificate chain but also bundles a jwks; one of the two is not what the operator meant (to pin a login issuer's leaf, use x5c_leaf_keys)", path, iss)
		}
		if len(entry.X5CLeafKeys) > 0 {
			if entry.Mechanism != MechanismX5C {
				return nil, fmt.Errorf("trust config %q: issuer %q pins leaf keys but does not resolve through a certificate chain, so there is no leaf to pin", path, iss)
			}
			if _, err := decodeLeafKeyPins(entry.X5CLeafKeys, iss); err != nil {
				return nil, fmt.Errorf("trust config %q: %w", path, err)
			}
		}
		// A login issuer terminates at its leaf (ADR-35), so the key it carries
		// has to be named here. Without it the credential would be checked
		// against a CA list instead — which admits whoever that CA vouched for,
		// and this deployment's operator never named them.
		if entry.Allows(PurposeLogin) && entry.Mechanism == MechanismX5C && len(entry.X5CLeafKeys) == 0 {
			return nil, fmt.Errorf("trust config %q: issuer %q is granted login and resolves by certificate chain, so it must pin the key its leaf carries in `x5c_leaf_keys`; a login issuer verified against a CA list would let that CA introduce another one", path, iss)
		}
		if entry.Mechanism == MechanismJWKS {
			if len(entry.JWKS) == 0 {
				return nil, fmt.Errorf("trust config %q: issuer %q uses mechanism jwks but bundles no keys", path, iss)
			}
			// Bundled keys are known now, so a set that cannot verify anything is
			// a configuration error to report on startup rather than at the first
			// credential.
			if _, err := signatureVerificationKeys(entry.JWKS, iss); err != nil {
				return nil, fmt.Errorf("trust config %q: %w", path, err)
			}
		}
		if entry.Mechanism == MechanismDIDJWK && !strings.HasPrefix(iss, "did:jwk:") {
			return nil, fmt.Errorf("trust config %q: issuer %q uses mechanism did:jwk but is not a did:jwk identifier", path, iss)
		}
		if entry.Mechanism == MechanismDIDWeb && !strings.HasPrefix(iss, "did:web:") {
			return nil, fmt.Errorf("trust config %q: issuer %q uses mechanism did:web but is not a did:web identifier", path, iss)
		}
		// A pid issuer attests a person, not a party, so it needs no
		// organizations. Anything that can speak for a party must say which.
		if !entry.Allows(PurposePID) && len(entry.Organizations) == 0 {
			return nil, fmt.Errorf("trust config %q: issuer %q may act for a party but lists no organizations", path, iss)
		}
	}

	// A policy that cannot be compiled stops the process here, where the error is
	// visible. Left to first use, a fat-fingered OID4VP_TRUST_POLICY_PATH
	// produced a service that came up healthy and then refused every login, peer
	// credential and PID, reporting each as an untrusted issuer.
	if err := PrepareTrustPolicy(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// For returns a view of this configuration restricted to one purpose. Key
// resolution (sdjwt) asks only "is this issuer trusted?", so scoping happens by
// handing it a view that answers that question for the purpose at hand.
func (c *TrustConfig) For(p Purpose) *PurposeView { return &PurposeView{cfg: c, purpose: p} }

// PurposeView is a TrustConfig restricted to a single purpose.
type PurposeView struct {
	cfg     *TrustConfig
	purpose Purpose
}

// IssuerTrusted reports whether this issuer is granted this purpose on the
// strength of configuration alone — no certificate chain has been validated.
//
// The rules are in policy/trust.rego, not here: which issuer may do what, on
// whose behalf, is the part a deployment changes, and it is easier to audit as
// data than as control flow. What stays in Go is everything cryptographic.
func (v *PurposeView) IssuerTrusted(iss string) bool {
	if v == nil || v.cfg == nil {
		return false
	}
	return v.cfg.evaluateBool(queryTrusted, v.purpose, iss, "", false)
}

// IssuerTrustedAnchored asks the same question having established the
// cryptographic fact an unlisted peer is admitted on (ADR-35): this credential's
// chain verified to this purpose's anchors, and its leaf named this issuer.
//
// It is a separate method rather than a parameter on IssuerTrusted so that a
// caller cannot assert the fact by accident. Only sdjwt's resolution path may
// pass it, because only there has the chain actually been checked.
func (v *PurposeView) IssuerTrustedAnchored(iss string) bool {
	if v == nil || v.cfg == nil {
		return false
	}
	return v.cfg.evaluateBool(queryTrusted, v.purpose, iss, "", true)
}

// IssuerPinnedLeafKeys returns the keys a login issuer's x5c leaf must carry
// (ADR-35), and whether this purpose pins the leaf at all.
//
// Only login does. A counterparty's Power of Attorney and a PID are verified to
// a CA root, because this deployment cannot know in advance which issuers will
// present them. A session here is the opposite question — the operator names
// their own organization's issuers — so it terminates at the leaf, and no CA in
// either list can introduce a new one.
//
// A login issuer that resolves by certificate chain and pins nothing is a
// configuration error, not an issuer that pins nothing: it would fall through to
// a CA check that is not the rule for login. LoadTrustConfig refuses it, and
// this returns the same error for a configuration assembled in code.
func (v *PurposeView) IssuerPinnedLeafKeys(iss string) ([][]byte, bool, error) {
	if v == nil || v.cfg == nil || v.purpose != PurposeLogin {
		return nil, false, nil
	}
	entry, ok := v.cfg.Issuers[canonicalIssuerKey(iss)]
	if !ok || entry.Mechanism != MechanismX5C {
		return nil, false, nil
	}
	if len(entry.X5CLeafKeys) == 0 {
		return nil, false, fmt.Errorf("login issuer %q resolves by certificate chain but pins no leaf key, so nothing states that this organization issued the credential", iss)
	}
	pins, err := decodeLeafKeyPins(entry.X5CLeafKeys, iss)
	if err != nil {
		return nil, false, err
	}
	return pins, true, nil
}

// decodeLeafKeyPins turns the configured base64 DER into the bytes a leaf's own
// SubjectPublicKeyInfo is compared against. Parsed rather than compared as text:
// base64 has more than one spelling of the same bytes, and a pin that missed
// because of padding would fail open into "no key matched" rather than loudly.
func decodeLeafKeyPins(encoded []string, iss string) ([][]byte, error) {
	pins := make([][]byte, 0, len(encoded))
	for i, value := range encoded {
		der, err := base64.StdEncoding.DecodeString(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("issuer %q: x5c_leaf_keys[%d] is not base64 DER: %w", iss, i, err)
		}
		if _, err := x509.ParsePKIXPublicKey(der); err != nil {
			return nil, fmt.Errorf("issuer %q: x5c_leaf_keys[%d] is not a DER SubjectPublicKeyInfo: %w", iss, i, err)
		}
		pins = append(pins, der)
	}
	return pins, nil
}

// IssuerListed reports whether the trust document carries an entry for this
// issuer at all. It is deliberately NOT a trust decision: it selects which
// resolution path runs. A listed issuer resolves by its declared mechanism; an
// unlisted one has only the anchored path, and reaches it before anything has
// been decided about it.
func (v *PurposeView) IssuerListed(iss string) bool {
	if v == nil || v.cfg == nil {
		return false
	}
	_, ok := v.cfg.Issuers[canonicalIssuerKey(iss)]
	return ok
}

// IssuerUsesX5C reports whether the issuer publishes its key through a
// certificate chain, so the configuration decides which resolution branch runs
// rather than the credential.
func (v *PurposeView) IssuerUsesX5C(iss string) (bool, error) {
	if !v.IssuerTrusted(iss) {
		if reasons := v.DenialReasons(iss, ""); len(reasons) > 0 {
			return false, fmt.Errorf("issuer %q is not trusted for %s: %s", iss, v.purpose, strings.Join(reasons, "; "))
		}
		return false, fmt.Errorf("issuer %q is not trusted for %s", iss, v.purpose)
	}
	return v.cfg.issuerUsesX5C(iss)
}

func (v *PurposeView) VCTAllowed(vct string) bool { return v.cfg.VCTAllowed(vct) }

func (v *PurposeView) IssuerJWKS(iss string) (json.RawMessage, error) {
	if !v.IssuerTrusted(iss) {
		return nil, fmt.Errorf("issuer %q is not trusted for %s", iss, v.purpose)
	}
	keys, err := v.cfg.resolveIssuerKeys(iss)
	if err != nil {
		return nil, err
	}
	return signatureVerificationKeys(keys, iss)
}

// signatureVerificationKeys narrows a resolved JWKS to the keys that may verify
// an issuer's signature.
//
// A JWKS states what its keys are for: `use: "enc"` publishes an encryption
// key, `key_ops` enumerates the operations allowed, and `alg` names the one
// algorithm the key is meant for. Reading only kty and crv, as verification
// did, let a key published for key agreement — our own gendid puts one in the
// DID document — verify credentials. The did:web path honours that separation
// through assertionMethod; a plain JWKS says it here, and it is equally
// binding.
//
// Keys are dropped rather than rejected wholesale: a JWKS legitimately carries
// a signing key and an encryption key side by side, and the signing one still
// has to work. An issuer left with nothing is an error, not a silent empty set.
func signatureVerificationKeys(raw json.RawMessage, iss string) (json.RawMessage, error) {
	// An x5c issuer resolves to no JWKS at all: its key arrives in the chain.
	if len(bytes.TrimSpace(raw)) == 0 {
		return raw, nil
	}

	var doc struct {
		Keys []json.RawMessage `json:"keys"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse jwks for issuer %q: %w", iss, err)
	}
	if len(doc.Keys) == 0 {
		return nil, fmt.Errorf("issuer %q resolved to an empty jwks", iss)
	}

	kept := make([]json.RawMessage, 0, len(doc.Keys))
	for _, key := range doc.Keys {
		var probe struct {
			Crv    string   `json:"crv"`
			Alg    string   `json:"alg"`
			Use    string   `json:"use"`
			KeyOps []string `json:"key_ops"`
		}
		if err := json.Unmarshal(key, &probe); err != nil {
			continue
		}
		if !keyVerifiesSignatures(probe.Use, probe.Alg, probe.Crv, probe.KeyOps) {
			continue
		}
		kept = append(kept, key)
	}
	if len(kept) == 0 {
		return nil, fmt.Errorf("issuer %q publishes no key usable for signature verification: every key is marked for encryption or declares an algorithm its curve cannot produce", iss)
	}

	out, err := json.Marshal(struct {
		Keys []json.RawMessage `json:"keys"`
	}{Keys: kept})
	if err != nil {
		return nil, fmt.Errorf("marshal jwks for issuer %q: %w", iss, err)
	}
	return out, nil
}

func keyVerifiesSignatures(use, alg, crv string, keyOps []string) bool {
	switch strings.ToLower(strings.TrimSpace(use)) {
	case "", "sig":
	default:
		return false
	}

	if len(keyOps) > 0 {
		verifies := false
		for _, op := range keyOps {
			if strings.EqualFold(strings.TrimSpace(op), "verify") {
				verifies = true
				break
			}
		}
		if !verifies {
			return false
		}
	}

	return signatureAlgMatchesCurve(alg, crv)
}

// signatureAlgMatchesCurve reports whether a key's declared alg is one its
// curve can produce. A P-256 key labelled ECDH-ES or RS256 was not published to
// verify credential signatures, whatever a credential header later claims. An
// absent alg says nothing and is left to the curve check in verification.
func signatureAlgMatchesCurve(alg, crv string) bool {
	alg = strings.ToUpper(strings.TrimSpace(alg))
	crv = strings.TrimSpace(crv)
	switch alg {
	case "":
		return true
	case "ES256":
		return crv == "" || crv == "P-256"
	case "ES384":
		return crv == "P-384"
	case "ES512":
		return crv == "P-521"
	}
	return false
}

// X5CTrustRoots returns the anchors for THIS view's purpose (ADR-35). The view
// already carries the purpose, so scoping the anchors needs no change to the
// interface sdjwt resolves against — a peer credential simply cannot be
// validated against the PID anchors, whatever it presents.
func (v *PurposeView) X5CTrustRoots() *x509.CertPool { return v.cfg.anchorsFor(v.purpose) }

// IssuerMayAttest reports whether the issuer was entitled to name this
// organization.
// IssuerMayAttest reports whether the issuer was entitled to name this
// organization, for the purpose the credential is being used for.
//
// The rule lives in policy/trust.rego. The purpose is passed rather than fixed
// so a deployment can make entitlement depend on it — separating "may authorize
// a signature here" from "may attest a peer's authority" is a policy edit, which
// is the point of the rules being data. Pinning it here would have made that
// impossible from the Go side regardless of what the policy said.
func (v *PurposeView) IssuerMayAttest(iss, org string) bool {
	if v == nil || v.cfg == nil {
		return false
	}
	// The anchored fact is not restated: this is only ever asked about a
	// credential that already verified, and for an unlisted issuer the only way
	// to have verified is the anchored path (policy/trust.rego, may_attest).
	return v.cfg.evaluateBool(queryMayAttest, v.purpose, iss, org, false)
}

func (c *TrustConfig) IssuerTrusted(iss string) bool {
	if c == nil {
		return false
	}
	_, ok := c.Issuers[canonicalIssuerKey(iss)]

	return ok
}

func (c *TrustConfig) VCTAllowed(vct string) bool {
	if c == nil {
		return false
	}

	vct = strings.TrimSpace(vct)

	for _, allowed := range c.VCTs {
		if vct == allowed {
			return true
		}
	}

	return false
}

func (c *TrustConfig) IssuerJWKS(iss string) (json.RawMessage, error) {
	entry, ok := c.Issuers[canonicalIssuerKey(iss)]
	if !ok {
		return nil, fmt.Errorf("issuer %q is not trusted", iss)
	}

	if len(entry.JWKS) == 0 {
		return nil, fmt.Errorf("issuer %q has no jwks", iss)
	}

	return signatureVerificationKeys(entry.JWKS, iss)
}

// anchorsFor returns the x5c chain-validation trust anchors for one purpose, or
// nil if that purpose has none (an x5c-bearing credential presented for it must
// then be refused, never accepted off its own embedded leaf cert — see
// sdjwt.verificationKeyFromX5C).
//
// There is deliberately no fallback to some other purpose's anchors, and no
// "default" set. A purpose whose anchors were forgotten must fail closed and
// say so; borrowing another purpose's would silently restore the flat pool
// ADR-35 replaced, at exactly the moment the configuration is wrong.
func (c *TrustConfig) anchorsFor(p Purpose) *x509.CertPool {
	if c == nil {
		return nil
	}
	return poolOf(c.x5cRoots[anchorSetFor(p)])
}

// anchorSetFor maps a purpose onto the CA trust list its chains verify against.
// There are TWO lists — PoA and PID — not one per purpose.
//
// A login issuer and a peer issuer are the same certificate seen from two
// sides. The Power of Attorney a holder obtains at login here travels inside the
// signed PDF to the counterparty, who verifies it as peer evidence against
// their PoA list — which must therefore contain the CA that issued OUR login
// issuer. Splitting login onto a third list would mean publishing the same root
// twice and keeping the copies in step.
//
// What separates login from peer is not the anchor but the enumeration: a login
// issuer must have an entry here, a peer need not (ADR-35). PID is the second
// list because a CA that attests persons must not be able to speak for a party,
// nor a party's CA attest a person — and because nobody can enumerate who
// issues a PID, including this deployment itself in production.
func anchorSetFor(p Purpose) Purpose {
	if p == PurposePID {
		return PurposePID
	}
	return PurposePeer
}

func poolOf(anchors []*x509.Certificate) *x509.CertPool {
	if len(anchors) == 0 {
		return nil
	}
	pool := x509.NewCertPool()
	for _, cert := range anchors {
		pool.AddCert(cert)
	}
	return pool
}

// SetX5CTrustRoots attaches the chain-validation anchors for one purpose,
// loaded via LoadX5CTrustAnchors. Separate from LoadTrustConfig because the
// anchors are PEM certificates, not JSON, and each purpose mounts its own
// (OID4VP_X5C_TRUST_ANCHORS_{PEER,PID,LOGIN}_PATH).
// Anchors ADD to the purpose rather than replacing it, because a purpose is
// legitimately fed from more than one place — a mounted bundle and, for `peer`,
// the root this deployment's own provisioning job minted.
func (c *TrustConfig) SetX5CTrustRoots(p Purpose, anchors []*x509.Certificate) {
	if len(anchors) == 0 {
		return
	}
	if c.x5cRoots == nil {
		c.x5cRoots = make(map[Purpose][]*x509.Certificate, 2)
	}
	set := anchorSetFor(p)
	c.x5cRoots[set] = append(c.x5cRoots[set], anchors...)
}

// unionX5CRoots is every anchor this deployment holds, for whatever purpose.
//
// The status-list verifier is one object for the process and is reached without
// a purpose in hand (checkStatusList), so it cannot be given a scoped set. The
// union is sound because the handlers now bind the list to the credential it
// governs (status.RequireCredentialIssuer): the list must be issued by the same
// issuer as that credential, and that credential was already admitted under its
// own purpose's anchors. So a list anchored for one purpose cannot speak for a
// credential admitted under another, which is the only thing a scoped set would
// have bought here.
func (c *TrustConfig) unionX5CRoots() *x509.CertPool {
	if c == nil {
		return nil
	}
	var all []*x509.Certificate
	for _, anchors := range c.x5cRoots {
		all = append(all, anchors...)
	}
	return poolOf(all)
}

// LoadX5CTrustAnchors reads a PEM bundle of one or more trusted root
// certificates (ConfigMap mount) that x5c-bearing credential chains must
// verify against.
//
// A bundle holds one root per issuer whose chains a deployment verifies — the
// PID issuer's, and the login issuer's, whose root is what a signed status list
// chains to. Every private key behind the shipped
// backend/config/oid4vp/x5c-trust-anchors.dev.pem is committed here, so anyone
// with a clone can issue under those anchors and be believed. Such an anchor is
// refused for the same reason a bundled JWKS key is, and certificates are
// therefore parsed one by one rather than handed to AppendCertsFromPEM, which
// reveals nothing about what it added — including a recognisable anchor sitting
// behind an unrecognised one.
// Certificates are returned rather than a finished pool so the caller can put
// the same anchor in more than one place: a purpose's own pool, and the union
// the status-list verifier holds. A CertPool cannot be enumerated once built,
// so a pool alone could not be combined with another.
func LoadX5CTrustAnchors(path string) ([]*x509.Certificate, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("x5c trust anchors path is empty")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read x5c trust anchors %q: %w", path, err)
	}

	allowDev := devTrustAllowed()
	var anchors []*x509.Certificate
	for rest := data; ; {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("x5c trust anchors %q: parse certificate: %w", path, err)
		}
		if !allowDev {
			if source, ok := devCertificateKey(cert); ok {
				return nil, fmt.Errorf("x5c trust anchors %q: anchor %q is keyed to material committed in this repository (%s), so anyone holding a clone could issue a certificate under it and be believed; set DCS_ALLOW_DEV_TRUST=true if this really is a development stack", path, cert.Subject.CommonName, source)
			}
		}
		anchors = append(anchors, cert)
	}

	if len(anchors) == 0 {
		return nil, fmt.Errorf("x5c trust anchors %q: no valid PEM certificates found", path)
	}

	return anchors, nil
}
