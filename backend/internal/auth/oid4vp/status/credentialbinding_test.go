package status_test

import (
	"testing"

	"digital-contracting-service/internal/auth/oid4vp/status"
)

func credentialFrom(claims map[string]any) status.VerifiedCredential {
	return status.VerifiedCredential{Claims: claims}
}

// The attack this closes: every issuer a deployment trusts is resolvable, so a
// signature on a status list says only that SOME trusted issuer published it.
// One of them could publish a list at another issuer's URI and un-revoke that
// issuer's credentials — signature valid, subject matching, and nothing having
// asked whose revocation statement it is (ADR-34).
func TestAnotherTrustedIssuersListCannotSpeakForThisCredential(t *testing.T) {
	credential := credentialFrom(map[string]any{"iss": "https://mine.example/issuer"})

	if err := status.RequireCredentialIssuer(credential, "https://mine.example/issuer"); err != nil {
		t.Fatalf("a credential's own issuer must be able to state its status: %v", err)
	}
	if err := status.RequireCredentialIssuer(credential, "https://someone-else.example/issuer"); err == nil {
		t.Fatal("a list from another issuer was accepted as this credential's revocation statement")
	}
}

// A W3C VC names its issuer `issuer`, not `iss`, and may carry it as an object.
// Reading only one spelling would silently bind nothing for the credentials this
// deployment issues about its own contracts.
func TestBothIssuerSpellingsAreRead(t *testing.T) {
	const issuer = "https://mine.example/issuer"
	for name, claims := range map[string]map[string]any{
		"sd-jwt iss": {"iss": issuer},
		"w3c string": {"issuer": issuer},
		"w3c object": {"issuer": map[string]any{"id": issuer}},
	} {
		if err := status.RequireCredentialIssuer(credentialFrom(claims), issuer); err != nil {
			t.Errorf("%s: issuer not read: %v", name, err)
		}
		if err := status.RequireCredentialIssuer(credentialFrom(claims), "https://other.example"); err == nil {
			t.Errorf("%s: a foreign list was accepted", name)
		}
	}
}

// A credential naming no issuer cannot exist — resolution needs `iss` to find a
// key at all — so reaching here without one means nothing was bound, and that
// must fail rather than read as "matches".
func TestCredentialWithoutAnIssuerIsRefused(t *testing.T) {
	if err := status.RequireCredentialIssuer(credentialFrom(nil), "https://mine.example/issuer"); err == nil {
		t.Fatal("a credential naming no issuer was bound to a list anyway")
	}
}

// A standard CWT status list need carry no `iss`, and its key is resolved by a
// lookup already scoped to the reference URI. Refusing it here would reject a
// conformant list over a claim its own format does not require.
func TestListWithoutAnIssuerIsLeftToItsOwnBinding(t *testing.T) {
	credential := credentialFrom(map[string]any{"iss": "https://mine.example/issuer"})
	if err := status.RequireCredentialIssuer(credential, ""); err != nil {
		t.Fatalf("a list declaring no issuer must be left to its URI-scoped key binding: %v", err)
	}
}

// The credential this deployment issues about its own contracts names its
// issuer as the instance did:web, while the status list that instance serves
// names its `iss` as the HTTPS origin that DID resolves from. did:web method
// §3 makes those one authority — the DID document and the list are served by
// the same origin — so the binding must recognise the two spellings, or every
// lifecycle credential this deployment issues is refused against its own list.
func TestDidWebIssuerMatchesItsOwnOrigin(t *testing.T) {
	for name, pair := range map[string]struct {
		credentialIssuer string
		listIssuer       string
	}{
		"host with port":     {"did:web:dcs-a.localhost%3A18080", "http://dcs-a.localhost:18080"},
		"https host":         {"did:web:mine.example", "https://mine.example"},
		"path segments":      {"did:web:mine.example:issuer", "https://mine.example/issuer"},
		"case-folded host":   {"did:web:Mine.Example", "https://mine.example"},
		"url names the list": {"https://mine.example/issuer", "https://mine.example/issuer"},
	} {
		credential := credentialFrom(map[string]any{"issuer": pair.credentialIssuer})
		if err := status.RequireCredentialIssuer(credential, pair.listIssuer); err != nil {
			t.Errorf("%s: %q refused against its own origin %q: %v", name, pair.credentialIssuer, pair.listIssuer, err)
		}
	}
}

// The equivalence is identity, not leniency: a did:web on one host never
// matches a list served from another, a path difference is a different issuer,
// and a scheme that is not a web origin matches nothing.
func TestDidWebIssuerRefusesForeignOrigins(t *testing.T) {
	for name, pair := range map[string]struct {
		credentialIssuer string
		listIssuer       string
	}{
		"different host":   {"did:web:mine.example", "https://someone-else.example"},
		"different port":   {"did:web:mine.example%3A8443", "https://mine.example:8080"},
		"different path":   {"did:web:mine.example:issuer", "https://mine.example/other"},
		"missing path":     {"did:web:mine.example:issuer", "https://mine.example"},
		"extra path":       {"did:web:mine.example", "https://mine.example/issuer"},
		"not a web origin": {"did:web:mine.example", "ftp://mine.example"},
		"not a did":        {"did:key:z6Mk", "https://mine.example"},
	} {
		credential := credentialFrom(map[string]any{"issuer": pair.credentialIssuer})
		if err := status.RequireCredentialIssuer(credential, pair.listIssuer); err == nil {
			t.Errorf("%s: %q accepted a list issued at %q", name, pair.credentialIssuer, pair.listIssuer)
		}
	}
}
