package oid4vp

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// devDIDDocument is shaped like this repository's own gendid output
// (backend/certs/dev/did-8991.json): the verification method is identified by a
// DID URL, while the JWK inside it carries a SHORT kid of its own. Both forms
// are in the document, and a credential may cite either.
const devDIDDocument = `{
  "@context": ["https://www.w3.org/ns/did/v1"],
  "id": "did:web:localhost%3A8991",
  "assertionMethod": [
    "did:web:localhost%3A8991#dev-key-1",
    "did:web:localhost%3A8991#dcs-vc"
  ],
  "verificationMethod": [
    {
      "controller": "did:web:localhost%3A8991",
      "id": "did:web:localhost%3A8991#dev-key-1",
      "type": "JsonWebKey2020",
      "publicKeyJwk": {
        "alg": "ES256",
        "crv": "P-256",
        "kid": "dev-key-1",
        "kty": "EC",
        "x": "VlBNhqQn6gLyQXqKkLDHBwXlJsi0IES4OovRv9FrAHI",
        "y": "vZMT1rkIeVaj7Om-FuIIcMHA1-xHtSk3OTGgovfeHCk"
      }
    },
    {
      "controller": "did:web:localhost%3A8991",
      "id": "did:web:localhost%3A8991#dcs-vc",
      "type": "JsonWebKey2020",
      "publicKeyJwk": {
        "alg": "ES256",
        "crv": "P-256",
        "kid": "dcs-vc",
        "kty": "EC",
        "x": "s7UdtIM60zJuEbVASvQJC0utyyDxbe1EdmMBlN2MRUc",
        "y": "d3pwxBZeRjZ5MePGlBiXRdK-Cb-u2H0t8HFhP26JVik"
      }
    }
  ]
}`

func resolveDevDIDKeys(t *testing.T) []map[string]any {
	t.Helper()

	const iss = "did:web:localhost%3A8991"
	cfg := &TrustConfig{
		VCTs: []string{"urn:dcs:poa:v1"},
		Issuers: map[string]TrustedIssuer{
			iss: {Purposes: []Purpose{PurposePeer}, Organizations: []string{"*"}, Mechanism: MechanismDIDWeb},
		},
	}
	cfg.SetKeyFetcher(stubFetcher{docs: map[string][]byte{
		"https://localhost:8991/.well-known/did.json": []byte(devDIDDocument),
	}})

	raw, err := cfg.For(PurposePeer).IssuerJWKS(iss)
	if err != nil {
		t.Fatalf("resolve %s: %v", iss, err)
	}
	var doc struct {
		Keys []map[string]any `json:"keys"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse resolved jwks: %v", err)
	}
	return doc.Keys
}

// A credential names the key that signed it by kid, and a DID document
// publishes TWO names for one key — the verification method's DID URL and the
// JWK's own kid inside it. This repository's gendid writes both, and an issuer
// may sign under either, so a credential citing either name must find the key.
//
// The match is made where the lookup happens (sdjwt.kidNamesKey) rather than by
// resolving each key twice under both names: a second entry per verification
// method would make a single-key document ambiguous for a credential that
// carries no kid at all, which resolves today.
func TestDIDWebKeyIsFoundByEitherKidForm(t *testing.T) {
	keys := resolveDevDIDKeys(t)

	// One entry per verification method — see above for why this matters.
	if len(keys) != 2 {
		t.Fatalf("resolved %d keys for a two-method document: %v", len(keys), kids(keys))
	}

	// Each key keeps the name the issuer signs under, and that name is the
	// fragment of the verification method's DID URL — which is what lets the
	// lookup accept either form. sdjwt.TestKidNamesKey covers the matching rule
	// itself.
	for _, want := range []string{"dev-key-1", "dcs-vc"} {
		found := false
		for _, kid := range kids(keys) {
			if kid == want {
				found = true
			}
		}
		if !found {
			t.Errorf("no key resolved under %q, the name its issuer signs with (resolved kids: %v)", want, kids(keys))
		}
	}
}

func kids(keys []map[string]any) []string {
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		kid, _ := key["kid"].(string)
		out = append(out, kid)
	}
	return out
}

// The gap above only exists because gendid writes a short kid INSIDE the JWK
// while identifying the verification method by DID URL. This pins that shape:
// if the generator ever stops doing it, the resolver's carry-over rule is
// answering a question nobody asks any more.
func TestGeneratedDIDDocumentNamesItsKeysTwice(t *testing.T) {
	data, err := os.ReadFile("../../../certs/dev/did-8991.json")
	if err != nil {
		t.Fatalf("read the checked-in dev identity: %v", err)
	}

	var doc struct {
		VerificationMethod []struct {
			ID           string `json:"id"`
			PublicKeyJWK struct {
				Kid string `json:"kid"`
			} `json:"publicKeyJwk"`
		} `json:"verificationMethod"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse the checked-in dev identity: %v", err)
	}
	if len(doc.VerificationMethod) == 0 {
		t.Fatal("the checked-in dev identity has no verification method")
	}

	for _, vm := range doc.VerificationMethod {
		if vm.PublicKeyJWK.Kid == "" {
			t.Errorf("verification method %q has no JWK kid", vm.ID)
			continue
		}
		if vm.PublicKeyJWK.Kid == vm.ID {
			t.Errorf("verification method %q names its key identically inside and outside; the two-name case this file exists for is gone", vm.ID)
		}
		if !strings.HasSuffix(vm.ID, "#"+vm.PublicKeyJWK.Kid) {
			t.Errorf("verification method %q does not end in its own JWK kid %q", vm.ID, vm.PublicKeyJWK.Kid)
		}
	}
}
