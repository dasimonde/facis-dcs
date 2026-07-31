package oid4vp

import (
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"digital-contracting-service/internal/auth/oid4vp/sdjwt"
)

// The shipped fixtures, addressed from this package's directory.
const (
	devTrustConfigPath = "../../../config/oid4vp/trust.dev.json"
	devX5CAnchorsPath  = "../../../config/oid4vp/x5c-trust-anchors.dev.pem"
)

// enforceDevKeyGuard makes the guard active regardless of the environment the
// test run inherits — a developer with DCS_ALLOW_DEV_TRUST exported would
// otherwise silently skip every assertion below.
func enforceDevKeyGuard(t *testing.T) {
	t.Helper()
	t.Setenv("DCS_ALLOW_DEV_TRUST", "")
}

func writeTrust(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "trust.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write trust config: %v", err)
	}
	return path
}

const jwksBlock = `{"keys":[{"kty":"EC","crv":"P-256","x":"VlBNhqQn6gLyQXqKkLDHBwXlJsi0IES4OovRv9FrAHI","y":"vZMT1rkIeVaj7Om-FuIIcMHA1-xHtSk3OTGgovfeHCk"}]}`

// The x-coordinate of testWallet/keys/issuer-dev.jwk, whose private half is
// committed to this repository.
const committedDevJWKS = `{"keys":[{"kty":"EC","crv":"P-256","x":"sAYnZiIkBGJWkgViAZy4Jsdsp3DXnL1mV7hYQKJYKss","y":"0e6ZLeEnI57444v4hIXDEvZQVgnxjFtv8-4oLqls3_o"}]}`

// A peer's issuer must not be usable to mint a session here: that is the whole
// reason trust is scoped by purpose rather than being one boolean.
func TestIssuerTrustedForOnlyTheGrantedPurpose(t *testing.T) {
	path := writeTrust(t, `{
      "vcts": ["urn:dcs:poa:v1"],
      "issuers": {
        "https://own.example/issuer": {
          "purposes": ["login","peer"],
          "organizations": ["did:web:own.example"],
          "mechanism": "jwks",
          "jwks": `+jwksBlock+`
        },
        "https://peer.example/issuer": {
          "purposes": ["peer"],
          "organizations": ["did:web:peer.example"],
          "mechanism": "jwks",
          "jwks": `+jwksBlock+`
        }
      }
    }`)

	cfg, err := LoadTrustConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if !cfg.For(PurposeLogin).IssuerTrusted("https://own.example/issuer") {
		t.Error("own issuer must be trusted for login")
	}
	if cfg.For(PurposeLogin).IssuerTrusted("https://peer.example/issuer") {
		t.Error("peer issuer must NOT be trusted for login")
	}
	if !cfg.For(PurposePeer).IssuerTrusted("https://peer.example/issuer") {
		t.Error("peer issuer must be trusted for peering")
	}
	// Mutual PoA binding: this instance verifies its own side too.
	if !cfg.For(PurposePeer).IssuerTrusted("https://own.example/issuer") {
		t.Error("own issuer must also be trusted for peering")
	}
	if _, err := cfg.For(PurposeLogin).IssuerJWKS("https://peer.example/issuer"); err == nil {
		t.Error("keys for an out-of-purpose issuer must not resolve")
	}
}

// An issuer may only speak for the organizations its entry names, so a trusted
// issuer cannot assert a party it was never entitled to represent.
func TestIssuerMayAttestOnlyListedOrganizations(t *testing.T) {
	path := writeTrust(t, `{
      "vcts": ["urn:dcs:poa:v1"],
      "issuers": {
        "https://peer.example/issuer": {
          "purposes": ["peer"],
          "organizations": ["did:web:peer.example"],
          "mechanism": "jwks",
          "jwks": `+jwksBlock+`
        }
      }
    }`)

	cfg, err := LoadTrustConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if !cfg.For(PurposePeer).IssuerMayAttest("https://peer.example/issuer", "did:web:peer.example") {
		t.Error("issuer must attest its own organization")
	}
	if cfg.For(PurposePeer).IssuerMayAttest("https://peer.example/issuer", "did:web:own.example") {
		t.Error("issuer must NOT attest an organization it does not hold")
	}
	if cfg.For(PurposePeer).IssuerMayAttest("https://peer.example/issuer", "") {
		t.Error("an empty organization must fail closed")
	}
	if cfg.For(PurposePeer).IssuerMayAttest("https://unknown.example/issuer", "did:web:peer.example") {
		t.Error("an unknown issuer must attest nothing")
	}
}

// Configuration that cannot be enforced is refused at load, not at first use:
// a deployment learns on startup, not when a wallet arrives.
func TestLoadRefusesUnenforceableEntries(t *testing.T) {
	cases := map[string]string{
		"no purposes":                        `{"purposes":[],"organizations":["did:web:a.example"],"mechanism":"jwks","jwks":` + jwksBlock + `}`,
		"unknown purpose":                    `{"purposes":["admin"],"organizations":["did:web:a.example"],"mechanism":"jwks","jwks":` + jwksBlock + `}`,
		"no mechanism":                       `{"purposes":["login"],"organizations":["did:web:a.example"],"jwks":` + jwksBlock + `}`,
		"unsupported mechanism":              `{"purposes":["login"],"organizations":["did:web:a.example"],"mechanism":"did:ebsi"}`,
		"jwks without keys":                  `{"purposes":["login"],"organizations":["did:web:a.example"],"mechanism":"jwks"}`,
		"party issuer without organizations": `{"purposes":["login"],"organizations":[],"mechanism":"jwks","jwks":` + jwksBlock + `}`,
	}

	for name, entry := range cases {
		t.Run(name, func(t *testing.T) {
			path := writeTrust(t, `{"vcts":["urn:dcs:poa:v1"],"issuers":{"https://a.example/issuer":`+entry+`}}`)
			if _, err := LoadTrustConfig(path); err == nil {
				t.Fatalf("expected %s to be refused at load", name)
			}
		})
	}
}

// A pid issuer attests a person rather than a party, so it needs no
// organizations — but it still may not stand in for one.
func TestPIDIssuerNeedsNoOrganizations(t *testing.T) {
	path := writeTrust(t, `{
      "vcts": ["urn:eudi:pid:de:1"],
      "issuers": {
        "https://pid.example/issuer": {
          "purposes": ["pid"],
          "mechanism": "jwks",
          "jwks": `+jwksBlock+`
        }
      }
    }`)

	cfg, err := LoadTrustConfig(path)
	if err != nil {
		t.Fatalf("a pid issuer without organizations must load: %v", err)
	}
	if cfg.For(PurposePeer).IssuerMayAttest("https://pid.example/issuer", "did:web:a.example") {
		t.Error("a pid issuer must not attest an organization")
	}
	if cfg.For(PurposeLogin).IssuerTrusted("https://pid.example/issuer") {
		t.Error("a pid issuer must not grant login")
	}
}

// The wildcard must be written out; it is not what an absent list means.
func TestOrganizationsWildcard(t *testing.T) {
	path := writeTrust(t, `{
      "vcts": ["urn:dcs:poa:v1"],
      "issuers": {
        "https://tenants.example/issuer": {
          "purposes": ["login"],
          "organizations": ["*"],
          "mechanism": "jwks",
          "jwks": `+jwksBlock+`
        }
      }
    }`)

	cfg, err := LoadTrustConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !cfg.For(PurposePeer).IssuerMayAttest("https://tenants.example/issuer", "Acme Corp") {
		t.Error("a wildcard issuer must attest any organization it names")
	}
	if cfg.For(PurposePeer).IssuerMayAttest("https://tenants.example/issuer", "") {
		t.Error("even a wildcard issuer must not attest an empty organization")
	}
}

func TestDevTrustConfigLoads(t *testing.T) {
	// The shipped fixture is keyed to committed material by design, so loading
	// it is exactly the case that now requires saying so.
	t.Setenv("DCS_ALLOW_DEV_TRUST", "true")
	cfg, err := LoadTrustConfig(devTrustConfigPath)
	if err != nil {
		t.Fatalf("shipped dev trust config must load: %v", err)
	}
	if len(cfg.Issuers) == 0 {
		t.Fatal("dev trust config has no issuers")
	}
	for iss, entry := range cfg.Issuers {
		if len(entry.Purposes) == 0 {
			t.Errorf("issuer %q has no purposes", iss)
		}
		// Only a mechanism that publishes keys in the entry has a JWKS to check.
		// An x5c issuer has none by design — its key arrives in the credential's
		// certificate chain — so demanding one of every entry made the mechanism
		// the configuration exists to choose unusable in the shipped file.
		switch entry.Mechanism {
		case MechanismX5C:
			if len(entry.JWKS) != 0 {
				t.Errorf("issuer %q resolves through a certificate chain but also bundles a jwks; one of the two is not what the operator meant", iss)
			}
		case MechanismJWKS:
			var probe map[string]any
			if err := json.Unmarshal(entry.JWKS, &probe); err != nil {
				t.Errorf("issuer %q jwks is not an object: %v", iss, err)
			}
		}
	}
}

type stubFetcher struct {
	docs map[string][]byte
	err  error
}

func (s stubFetcher) Fetch(url string) ([]byte, error) {
	if s.err != nil {
		return nil, s.err
	}
	body, ok := s.docs[url]
	if !ok {
		return nil, fmt.Errorf("no document at %s", url)
	}
	return body, nil
}

// did:web resolution must hit the identifier's own document, with the port
// decoded back out of the percent-encoded authority.
func TestDIDWebURLMapping(t *testing.T) {
	cases := map[string]string{
		"did:web:example.com":                    "https://example.com/.well-known/did.json",
		"did:web:example.com:issuer":             "https://example.com/issuer/did.json",
		"did:web:dcs-b.localhost%3A18080:issuer": "https://dcs-b.localhost:18080/issuer/did.json",
	}
	for iss, want := range cases {
		got, err := didWebURL(iss)
		if err != nil {
			t.Errorf("%s: %v", iss, err)
			continue
		}
		if got != want {
			t.Errorf("%s → %s, want %s", iss, got, want)
		}
	}
	if _, err := didWebURL("https://example.com/issuer"); err == nil {
		t.Error("a non did:web identifier must be refused")
	}
}

func TestResolveKeysByMechanism(t *testing.T) {
	// A realistic document: it names itself, and separates the key that may make
	// assertions from a key-agreement key that may not.
	didDoc := []byte(`{
      "id": "did:web:example.com:issuer",
      "verificationMethod": [
        {"id": "did:web:example.com:issuer#key-1", "publicKeyJwk": {"kty":"EC","crv":"P-256","x":"VlBNhqQn6gLyQXqKkLDHBwXlJsi0IES4OovRv9FrAHI","y":"vZMT1rkIeVaj7Om-FuIIcMHA1-xHtSk3OTGgovfeHCk"}},
        {"id": "did:web:example.com:issuer#dcs-ecdh", "publicKeyJwk": {"kty":"EC","crv":"P-256","x":"s7UdtIM60zJuEbVASvQJC0utyyDxbe1EdmMBlN2MRUc","y":"d3pwxBZeRjZ5MePGlBiXRdK-Cb-u2H0t8HFhP26JVik"}}
      ],
      "assertionMethod": ["did:web:example.com:issuer#key-1"],
      "keyAgreement": ["did:web:example.com:issuer#dcs-ecdh"]
    }`)
	cfg := &TrustConfig{
		VCTs: []string{"urn:dcs:poa:v1"},
		Issuers: map[string]TrustedIssuer{
			"did:web:example.com:issuer":  {Purposes: []Purpose{PurposePeer}, Organizations: []string{"*"}, Mechanism: MechanismDIDWeb},
			"https://x5c.example/issuer":  {Purposes: []Purpose{PurposePID}, Mechanism: MechanismX5C},
			"https://orce.example/issuer": {Purposes: []Purpose{PurposePeer}, Organizations: []string{"*"}, Mechanism: MechanismORCE},
		},
	}
	cfg.SetKeyFetcher(stubFetcher{docs: map[string][]byte{
		"https://example.com/issuer/did.json": didDoc,
	}})

	keys, err := cfg.resolveIssuerKeys("did:web:example.com:issuer")
	if err != nil {
		t.Fatalf("did:web resolve: %v", err)
	}
	if !strings.Contains(string(keys), "VlBNhqQn6gLy") {
		t.Errorf("did:web assertionMethod key not returned: %s", keys)
	}
	// The key-agreement key is published in the same document and must NOT be
	// usable to verify signatures: a DID document states that separation and a
	// resolver that ignores it reuses an encryption key for assertions.
	if strings.Contains(string(keys), "s7UdtIM60zJu") {
		t.Error("the keyAgreement key must not enter the issuer JWKS")
	}

	// An x5c issuer resolves to no JWKS: its key arrives in the chain.
	keys, err = cfg.resolveIssuerKeys("https://x5c.example/issuer")
	if err != nil || len(keys) != 0 {
		t.Errorf("x5c must resolve to no jwks, got %q err %v", keys, err)
	}

	// orce without a configured endpoint must say so, not fail obscurely.
	if _, err := cfg.resolveIssuerKeys("https://orce.example/issuer"); err == nil ||
		!strings.Contains(err.Error(), "no resolver endpoint is configured") {
		t.Errorf("expected a clear orce configuration error, got %v", err)
	}
}

// Federation cannot require editing every instance's trust file whenever a
// member is onboarded, so a peer's issuer is trusted dynamically — bounded by
// its own authority, and authorized separately by the ADR-19 gate and the PDP.
func TestDynamicPeerTrust(t *testing.T) {
	cfg := &TrustConfig{
		VCTs:        []string{"urn:dcs:poa:v1"},
		PeerDynamic: true,
		Issuers: map[string]TrustedIssuer{
			"https://own.example/issuer": {
				Purposes: []Purpose{PurposeLogin}, Organizations: []string{"did:web:own.example"},
				Mechanism: MechanismJWKS, JWKS: json.RawMessage(jwksBlock),
			},
		},
	}

	unlisted := "did:web:newpeer.example:issuer"

	if !cfg.For(PurposePeer).IssuerTrusted(unlisted) {
		t.Error("an unlisted did:web peer issuer must be verifiable for peering")
	}
	// Trusting it is worthless if no key can be resolved for it. Asserting only
	// the gate let this ship as dead code: IssuerTrusted said yes while
	// resolution rejected the same issuer, so no dynamic peer could ever be
	// verified.
	cfg.SetKeyFetcher(stubFetcher{docs: map[string][]byte{
		"https://newpeer.example/issuer/did.json": []byte(`{
          "id": "did:web:newpeer.example:issuer",
          "verificationMethod": [{"id": "did:web:newpeer.example:issuer#key-1", "publicKeyJwk": {"kty":"EC","crv":"P-256","x":"VlBNhqQn6gLyQXqKkLDHBwXlJsi0IES4OovRv9FrAHI","y":"vZMT1rkIeVaj7Om-FuIIcMHA1-xHtSk3OTGgovfeHCk"}}],
          "assertionMethod": ["did:web:newpeer.example:issuer#key-1"]
        }`),
	}})
	keys, err := cfg.For(PurposePeer).IssuerJWKS(unlisted)
	if err != nil || !strings.Contains(string(keys), "VlBNhqQn6gLy") {
		t.Fatalf("a dynamic peer's key must resolve from its own DID document: keys=%q err=%v", keys, err)
	}
	if usesX5C, err := cfg.For(PurposePeer).IssuerUsesX5C(unlisted); err != nil || usesX5C {
		t.Errorf("a dynamic peer publishes a DID document, not a chain: %v %v", usesX5C, err)
	}
	// Access to this deployment stays the operator's explicit decision.
	if cfg.For(PurposeLogin).IssuerTrusted(unlisted) {
		t.Error("a dynamic peer issuer must NOT grant login")
	}
	if cfg.For(PurposePID).IssuerTrusted(unlisted) {
		t.Error("a dynamic peer issuer must NOT serve as a PID issuer")
	}
	// It speaks for its own party and no other.
	if !cfg.For(PurposePeer).IssuerMayAttest(unlisted, "did:web:newpeer.example") {
		t.Error("a peer issuer must attest its own authority")
	}
	if cfg.For(PurposePeer).IssuerMayAttest(unlisted, "did:web:own.example") {
		t.Error("a peer issuer must not attest another party")
	}

	// Without the flag, nothing is trusted implicitly.
	cfg.PeerDynamic = false
	if cfg.For(PurposePeer).IssuerTrusted(unlisted) {
		t.Error("dynamic peer trust must be opt-in")
	}
}

// A dynamically trusted issuer speaks for its own authority and no other. The
// bound used to be derived by a Go helper that duplicated the policy's
// peer_authority rule; it is asserted here through the decision it governs, so
// there is one statement of it rather than two that can drift.
func TestDynamicPeerAttestsOnlyItsOwnAuthority(t *testing.T) {
	cfg := &TrustConfig{PeerDynamic: true}

	cases := map[string]string{
		"did:web:example.com:issuer":             "did:web:example.com",
		"did:web:example.com":                    "did:web:example.com",
		"did:web:dcs-b.localhost%3A18080:issuer": "did:web:dcs-b.localhost%3A18080",
	}
	for iss, authority := range cases {
		if !cfg.For(PurposePeer).IssuerMayAttest(iss, authority) {
			t.Errorf("%s may not attest its own authority %q", iss, authority)
		}
		if cfg.For(PurposePeer).IssuerMayAttest(iss, "did:web:somewhere-else.example") {
			t.Errorf("%s attested an authority that is not its own", iss)
		}
	}

	// Only did:web qualifies: an https issuer has no authority to derive.
	if cfg.For(PurposePeer).IssuerMayAttest("https://example.com/issuer", "https://example.com") {
		t.Error("a non-did:web issuer was dynamically entitled to an organization")
	}
}

// The configuration decides how an issuer's key is resolved. If the credential
// decided, anyone holding a certificate under any configured anchor could
// present it for an issuer that publishes a JWKS and be believed.
func TestMechanismIsAuthoritativeNotTheCredential(t *testing.T) {
	path := writeTrust(t, `{
      "vcts": ["urn:dcs:poa:v1"],
      "issuers": {
        "https://jwks.example/issuer": {
          "purposes": ["login"], "organizations": ["did:web:jwks.example"],
          "mechanism": "jwks", "jwks": `+jwksBlock+`
        },
        "https://chain.example/issuer": {
          "purposes": ["pid"], "mechanism": "x5c"
        }
      }
    }`)
	cfg, err := LoadTrustConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	usesX5C, err := cfg.For(PurposeLogin).IssuerUsesX5C("https://jwks.example/issuer")
	if err != nil || usesX5C {
		t.Errorf("a jwks issuer must not be resolvable through a chain: %v %v", usesX5C, err)
	}
	usesX5C, err = cfg.For(PurposePID).IssuerUsesX5C("https://chain.example/issuer")
	if err != nil || !usesX5C {
		t.Errorf("an x5c issuer must resolve through its chain: %v %v", usesX5C, err)
	}
	// Out of purpose, the question is not answerable at all.
	if _, err := cfg.For(PurposeLogin).IssuerUsesX5C("https://chain.example/issuer"); err == nil {
		t.Error("mechanism must not be resolvable for an issuer outside its purpose")
	}
}

// An explicit entry is the operator's complete answer: withholding a purpose
// denies it, rather than falling through to the dynamic peer path.
func TestExplicitEntryDeniesRatherThanFallingThrough(t *testing.T) {
	cfg := &TrustConfig{
		VCTs:        []string{"urn:dcs:poa:v1"},
		PeerDynamic: true,
		Issuers: map[string]TrustedIssuer{
			"did:web:listed.example:issuer": {
				Purposes: []Purpose{PurposeLogin}, Organizations: []string{"did:web:listed.example"},
				Mechanism: MechanismJWKS, JWKS: json.RawMessage(jwksBlock),
			},
		},
	}
	if cfg.For(PurposePeer).IssuerTrusted("did:web:listed.example:issuer") {
		t.Error("an entry granting only login must not also grant peer via the dynamic path")
	}
}

// The dev fixture is baked into the runtime image and is the chart default, so
// a deployment could trust an issuer whose private key ships in this repo.
func TestCommittedDevKeyIsRefusedUnlessExplicitlyAllowed(t *testing.T) {
	body := `{"vcts":["urn:dcs:poa:v1"],"issuers":{"did:web:dev.example:issuer:poa":{
      "purposes":["login"],"organizations":["*"],"mechanism":"jwks","jwks":` + committedDevJWKS + `}}}`

	path := writeTrust(t, body)
	if _, err := LoadTrustConfig(path); err == nil {
		t.Fatal("a trust config keyed to repo-committed material must be refused")
	} else if !strings.Contains(err.Error(), "committed in this repository") {
		t.Errorf("the refusal must say why: %v", err)
	}

	// The dev and CI stacks legitimately use it, and say so out loud.
	t.Setenv("DCS_ALLOW_DEV_TRUST", "true")
	if _, err := LoadTrustConfig(writeTrust(t, body)); err != nil {
		t.Fatalf("DCS_ALLOW_DEV_TRUST must permit the dev fixture: %v", err)
	}
}

// The guard is only worth anything if it fires on the file that actually
// ships. Asserting it against a fixture written by the test, while the one
// test that reads the real file disables the guard first, left the shipped
// configuration unexamined — which is the exact thing that reaches a
// production install by omission.
func TestShippedDevTrustConfigIsRefusedWithoutDevTrust(t *testing.T) {
	enforceDevKeyGuard(t)

	_, err := LoadTrustConfig(devTrustConfigPath)
	if err == nil {
		t.Fatal("the shipped dev trust config is keyed to committed material and must be refused unless DCS_ALLOW_DEV_TRUST says otherwise")
	}
	if !strings.Contains(err.Error(), "committed in this repository") {
		t.Errorf("the refusal must say why: %v", err)
	}
}

// Every private key in this repository, not the one that was noticed first.
func TestDevKeyGuardCoversEveryCommittedKey(t *testing.T) {
	enforceDevKeyGuard(t)

	for x, source := range devIssuerKeySources {
		t.Run(source, func(t *testing.T) {
			body := fmt.Sprintf(`{"vcts":["urn:dcs:poa:v1"],"issuers":{"https://a.example/issuer":{
              "purposes":["login"],"organizations":["*"],"mechanism":"jwks",
              "jwks":{"keys":[{"kty":"EC","crv":"P-256","x":%q,"y":"0e6ZLeEnI57444v4hIXDEvZQVgnxjFtv8-4oLqls3_o"}]}}}}`, x)
			if _, err := LoadTrustConfig(writeTrust(t, body)); err == nil {
				t.Fatalf("%s is committed here and must not be trustable as an issuer key", source)
			}
		})
	}
}

// Verification decodes a coordinate into a big.Int, so a leading-zero-padded
// encoding of a committed key IS that key. Comparing the base64 text let the
// same private key be configured under a spelling the guard had never seen.
func TestDevKeyGuardComparesKeysNotEncodings(t *testing.T) {
	enforceDevKeyGuard(t)

	const committed = "sAYnZiIkBGJWkgViAZy4Jsdsp3DXnL1mV7hYQKJYKss"
	raw, err := base64.RawURLEncoding.DecodeString(committed)
	if err != nil {
		t.Fatalf("decode committed coordinate: %v", err)
	}
	padded := base64.RawURLEncoding.EncodeToString(append([]byte{0x00}, raw...))
	if padded == committed {
		t.Fatal("the padded encoding must differ textually, or this proves nothing")
	}

	body := fmt.Sprintf(`{"vcts":["urn:dcs:poa:v1"],"issuers":{"https://a.example/issuer":{
      "purposes":["login"],"organizations":["*"],"mechanism":"jwks",
      "jwks":{"keys":[{"kty":"EC","crv":"P-256","x":%q,"y":"0e6ZLeEnI57444v4hIXDEvZQVgnxjFtv8-4oLqls3_o"}]}}}}`, padded)
	if _, err := LoadTrustConfig(writeTrust(t, body)); err == nil {
		t.Fatal("a re-encoding of committed key material is the same key and must be refused")
	}
}

// The same private key reaches the verifier just as well through a did:jwk
// identifier or a certificate, so a guard that reads only `jwks` was evadable
// by writing the key down differently.
func TestDevKeyGuardSeesDIDJWKAndCertificateForms(t *testing.T) {
	enforceDevKeyGuard(t)

	t.Run("did:jwk", func(t *testing.T) {
		iss, err := sdjwt.DIDJWKFromPublicJWK(sdjwt.JWK{
			Kty: "EC", Crv: "P-256",
			X: "sAYnZiIkBGJWkgViAZy4Jsdsp3DXnL1mV7hYQKJYKss",
			Y: "0e6ZLeEnI57444v4hIXDEvZQVgnxjFtv8-4oLqls3_o",
		})
		if err != nil {
			t.Fatalf("build did:jwk: %v", err)
		}
		body := fmt.Sprintf(`{"vcts":["urn:dcs:poa:v1"],"issuers":{%q:{
          "purposes":["login"],"organizations":["*"],"mechanism":"did:jwk"}}}`, iss)
		if _, err := LoadTrustConfig(writeTrust(t, body)); err == nil {
			t.Fatal("a did:jwk issuer that IS committed key material must be refused")
		}
	})

	t.Run("x5c member", func(t *testing.T) {
		body := fmt.Sprintf(`{"vcts":["urn:dcs:poa:v1"],"issuers":{"https://a.example/issuer":{
          "purposes":["pid"],"mechanism":"jwks",
          "jwks":{"keys":[{"kty":"EC","crv":"P-256",
            "x":"VlBNhqQn6gLyQXqKkLDHBwXlJsi0IES4OovRv9FrAHI",
            "y":"vZMT1rkIeVaj7Om-FuIIcMHA1-xHtSk3OTGgovfeHCk",
            "x5c":[%q]}]}}}}`, devAnchorCertificateB64(t))
		if _, err := LoadTrustConfig(writeTrust(t, body)); err == nil {
			t.Fatal("a certificate carrying committed key material must be refused however it is bundled")
		}
	})
}

// The dev CA is self-signed with testWallet/keys/issuer-dev-x5c.jwk, so
// configuring it as an anchor lets anyone with a clone issue a certificate
// under it and be believed as a PID issuer — the same hazard as a bundled key,
// one indirection further out.
func TestDevX5CTrustAnchorsRefusedUnlessExplicitlyAllowed(t *testing.T) {
	enforceDevKeyGuard(t)

	_, err := LoadX5CTrustAnchors(devX5CAnchorsPath)
	if err == nil {
		t.Fatal("the shipped dev CA must not be usable as a trust anchor without DCS_ALLOW_DEV_TRUST")
	}
	if !strings.Contains(err.Error(), "committed in this repository") {
		t.Errorf("the refusal must say why: %v", err)
	}

	t.Setenv("DCS_ALLOW_DEV_TRUST", "true")
	pool, err := LoadX5CTrustAnchors(devX5CAnchorsPath)
	if err != nil || pool == nil {
		t.Fatalf("DCS_ALLOW_DEV_TRUST must permit the dev anchors: %v", err)
	}
}

func devAnchorCertificateB64(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(devX5CAnchorsPath)
	if err != nil {
		t.Fatalf("read dev anchors: %v", err)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		t.Fatal("dev anchors hold no PEM certificate")
	}
	return base64.StdEncoding.EncodeToString(block.Bytes)
}

// A JWKS says what its keys are for. Reading only kty and crv let a key
// published for encryption verify credential signatures — the separation the
// did:web path honours through assertionMethod, ignored the moment the same
// key set arrives as a plain JWKS.
func TestKeysPublishedForEncryptionDoNotVerifySignatures(t *testing.T) {
	const signing = `{"kty":"EC","crv":"P-256","kid":"sig","x":"VlBNhqQn6gLyQXqKkLDHBwXlJsi0IES4OovRv9FrAHI","y":"vZMT1rkIeVaj7Om-FuIIcMHA1-xHtSk3OTGgovfeHCk"}`
	const encryption = `{"kty":"EC","crv":"P-256","kid":"enc","use":"enc","x":"s7UdtIM60zJuEbVASvQJC0utyyDxbe1EdmMBlN2MRUc","y":"d3pwxBZeRjZ5MePGlBiXRdK-Cb-u2H0t8HFhP26JVik"}`
	const wrongAlg = `{"kty":"EC","crv":"P-256","kid":"ecdh","alg":"ECDH-ES","x":"s7UdtIM60zJuEbVASvQJC0utyyDxbe1EdmMBlN2MRUc","y":"d3pwxBZeRjZ5MePGlBiXRdK-Cb-u2H0t8HFhP26JVik"}`
	const p384Alg = `{"kty":"EC","crv":"P-256","kid":"es384","alg":"ES384","x":"s7UdtIM60zJuEbVASvQJC0utyyDxbe1EdmMBlN2MRUc","y":"d3pwxBZeRjZ5MePGlBiXRdK-Cb-u2H0t8HFhP26JVik"}`
	const encOps = `{"kty":"EC","crv":"P-256","kid":"ops","key_ops":["deriveKey"],"x":"s7UdtIM60zJuEbVASvQJC0utyyDxbe1EdmMBlN2MRUc","y":"d3pwxBZeRjZ5MePGlBiXRdK-Cb-u2H0t8HFhP26JVik"}`

	cfg := &TrustConfig{
		VCTs: []string{"urn:dcs:poa:v1"},
		Issuers: map[string]TrustedIssuer{
			"https://mixed.example/issuer": {
				Purposes: []Purpose{PurposeLogin}, Organizations: []string{"*"}, Mechanism: MechanismJWKS,
				JWKS: json.RawMessage(`{"keys":[` + signing + `,` + encryption + `,` + wrongAlg + `,` + p384Alg + `,` + encOps + `]}`),
			},
		},
	}

	keys, err := cfg.For(PurposeLogin).IssuerJWKS("https://mixed.example/issuer")
	if err != nil {
		t.Fatalf("the signing key must still resolve: %v", err)
	}
	if !strings.Contains(string(keys), `"sig"`) {
		t.Errorf("the signing key was dropped: %s", keys)
	}
	for _, dropped := range []string{`"enc"`, `"ecdh"`, `"es384"`, `"ops"`} {
		if strings.Contains(string(keys), dropped) {
			t.Errorf("key %s is not published for signature verification and must not be offered for it: %s", dropped, keys)
		}
	}

	// An issuer left with nothing usable is a refusal, not an empty key set that
	// reads as "no matching kid" three layers down.
	cfg.Issuers["https://enconly.example/issuer"] = TrustedIssuer{
		Purposes: []Purpose{PurposeLogin}, Organizations: []string{"*"}, Mechanism: MechanismJWKS,
		JWKS: json.RawMessage(`{"keys":[` + encryption + `]}`),
	}
	if _, err := cfg.For(PurposeLogin).IssuerJWKS("https://enconly.example/issuer"); err == nil {
		t.Error("an issuer publishing only encryption keys must not resolve to a usable JWKS")
	}
}

// Bundled keys are known at load, so a set that can verify nothing is a
// startup error rather than a puzzle when the first credential arrives.
func TestLoadRefusesJWKSThatCanVerifyNothing(t *testing.T) {
	path := writeTrust(t, `{"vcts":["urn:dcs:poa:v1"],"issuers":{"https://a.example/issuer":{
      "purposes":["login"],"organizations":["*"],"mechanism":"jwks",
      "jwks":{"keys":[{"kty":"EC","crv":"P-256","use":"enc","x":"VlBNhqQn6gLyQXqKkLDHBwXlJsi0IES4OovRv9FrAHI","y":"vZMT1rkIeVaj7Om-FuIIcMHA1-xHtSk3OTGgovfeHCk"}]}}}}`)
	if _, err := LoadTrustConfig(path); err == nil {
		t.Fatal("an issuer whose only bundled key is an encryption key must be refused at load")
	}
}
