package sdjwt

import (
	"encoding/base64"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func disclosureFor(t *testing.T, name string, value any) string {
	t.Helper()
	raw, err := json.Marshal([]any{"salt", name, value})
	if err != nil {
		t.Fatalf("marshal disclosure: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

// arrayElementDisclosureFor builds the two-element disclosure RFC 9901 §4.2.4.2
// defines for one hidden element of an array.
func arrayElementDisclosureFor(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal([]any{"salt-" + strings.TrimSpace(reflect.TypeOf(value).String()), value})
	if err != nil {
		t.Fatalf("marshal array disclosure: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func sdArray(disclosures ...string) []any {
	digests := make([]any, 0, len(disclosures))
	for _, d := range disclosures {
		digests = append(digests, disclosureDigest(d))
	}
	return digests
}

func placeholder(disclosure string) map[string]any {
	return map[string]any{arrayElementPlaceholderKey: disclosureDigest(disclosure)}
}

// The registered claims are checked against the raw signed payload, but `iss`,
// `cnf` and `status` are read again from the merged map — to pick the issuer
// entry whose organization entitlement is checked, to bind the holder, and to
// find the revocation list. A disclosure carrying one of those would move a
// check to a target the issuer never signed for.
func TestDisclosureCannotCarryARegisteredClaim(t *testing.T) {
	for _, name := range []string{"iss", "sub", "cnf", "status", "vct", "exp", "_sd", "_sd_alg"} {
		t.Run(name, func(t *testing.T) {
			d := disclosureFor(t, name, "attacker-chosen")
			_, err := MergeDisclosedClaims(
				jwt.MapClaims{"iss": "did:web:real.example", "_sd": sdArray(d)},
				[]string{d},
			)
			if err == nil {
				t.Fatalf("a disclosure carrying %q was merged", name)
			}
			if !strings.Contains(err.Error(), "registered claim") {
				t.Fatalf("refused for the wrong reason: %v", err)
			}
		})
	}
}

func TestDisclosureCannotOverrideASignedClaim(t *testing.T) {
	d := disclosureFor(t, "organization", "Someone Else")
	_, err := MergeDisclosedClaims(
		jwt.MapClaims{"organization": "Acme Corp", "_sd": sdArray(d)},
		[]string{d},
	)
	if err == nil {
		t.Fatal("a disclosure overrode a claim the issuer signed")
	}
	if !strings.Contains(err.Error(), "overrides a claim") {
		t.Fatalf("refused for the wrong reason: %v", err)
	}
}

// The ordinary case still works: a selectively disclosed claim the issuer left
// out of the payload merges in.
func TestDisclosedClaimStillMerges(t *testing.T) {
	d := disclosureFor(t, "organization", "Acme Corp")
	claims, err := MergeDisclosedClaims(
		jwt.MapClaims{"iss": "did:web:real.example", "_sd": sdArray(d)},
		[]string{d},
	)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if claims["organization"] != "Acme Corp" {
		t.Fatalf("organization did not merge: %v", claims)
	}
}

// A disclosure nothing in the payload asks for is refused: the issuer signed no
// digest for it, so its value is unattested.
func TestUnsolicitedDisclosureIsRefused(t *testing.T) {
	_, err := MergeDisclosedClaims(
		jwt.MapClaims{"iss": "did:web:real.example"},
		[]string{disclosureFor(t, "organization", "Acme Corp")},
	)
	if err == nil {
		t.Fatal("a disclosure the credential never listed was merged")
	}
}

// Any issuer of an array-valued selectively disclosable claim emits two-element
// disclosures; the elements the holder withheld drop out of the array.
func TestArrayElementDisclosuresResolve(t *testing.T) {
	signer := arrayElementDisclosureFor(t, "Contract Signer")
	approver := arrayElementDisclosureFor(t, "Contract Approver")

	claims, err := MergeDisclosedClaims(
		jwt.MapClaims{
			"iss":   "did:web:real.example",
			"roles": []any{"Contract Manager", placeholder(signer), placeholder(approver)},
		},
		[]string{signer},
	)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}

	want := []any{"Contract Manager", "Contract Signer"}
	if !reflect.DeepEqual(claims["roles"], want) {
		t.Fatalf("roles resolved to %#v, want %#v", claims["roles"], want)
	}
}

// _sd may appear in any object, not only the payload root — a credential whose
// disclosable claims are all nested carries no top-level _sd at all.
func TestNestedSDDigestsResolve(t *testing.T) {
	locality := disclosureFor(t, "locality", "Berlin")
	nested := disclosureFor(t, "address", map[string]any{"_sd": sdArray(locality)})

	claims, err := MergeDisclosedClaims(
		jwt.MapClaims{"iss": "did:web:real.example", "_sd": sdArray(nested)},
		[]string{nested, locality},
	)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}

	address, ok := claims["address"].(map[string]any)
	if !ok {
		t.Fatalf("address did not resolve to an object: %#v", claims["address"])
	}
	if address["locality"] != "Berlin" {
		t.Fatalf("nested locality did not resolve: %#v", address)
	}
	if _, leaked := address["_sd"]; leaked {
		t.Fatalf("the resolved object still carries _sd: %#v", address)
	}
}

// A credential whose only disclosable claims are nested has no top-level _sd,
// which must not be read as "this credential discloses nothing".
func TestCredentialWithoutTopLevelSDIsAccepted(t *testing.T) {
	locality := disclosureFor(t, "locality", "Berlin")

	claims, err := MergeDisclosedClaims(
		jwt.MapClaims{
			"iss":     "did:web:real.example",
			"address": map[string]any{"_sd": sdArray(locality)},
		},
		[]string{locality},
	)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	address, _ := claims["address"].(map[string]any)
	if address["locality"] != "Berlin" {
		t.Fatalf("nested locality did not resolve: %#v", claims["address"])
	}
}
