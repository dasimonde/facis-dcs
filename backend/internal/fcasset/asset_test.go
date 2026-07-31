package fcasset

import (
	"testing"
	"time"
)

// The VC 2.0 context defines issuer, validFrom and credentialSubject only
// inside VerifiableCredential's scoped @context. Removing the type therefore
// does not merely relabel the document — JSON-LD expansion drops every one of
// those terms, FC ingests a node with no claims, and publish still answers 200.
// That failure is silent on both sides, so it is pinned here rather than left
// to be rediscovered from an empty catalogue.
func TestPayloadStaysTypedSoItsClaimsSurviveExpansion(t *testing.T) {
	payload, err := BuildPayload(BuildInput{
		Issuer:             "did:web:dev.example",
		SubjectIRI:         "https://dev.example/templates/1",
		ValidFrom:          time.Now().UTC(),
		Subject:            CatalogueSubject{ID: "1", Name: "Lease"},
		TemplateDataString: "{}",
	})
	if err != nil {
		t.Fatalf("BuildPayload: %v", err)
	}

	types, ok := payload["type"].([]string)
	if !ok {
		t.Fatalf("type is %T, want []string", payload["type"])
	}
	var typed bool
	for _, v := range types {
		if v == "VerifiableCredential" {
			typed = true
		}
	}
	if !typed {
		t.Fatalf("type %v omits VerifiableCredential: credentialSubject, issuer and "+
			"validFrom are scoped to it in the VC 2.0 context and expand to nothing without it", types)
	}

	for _, term := range []string{"issuer", "validFrom", "credentialSubject"} {
		if _, ok := payload[term]; !ok {
			t.Errorf("payload carries no %q", term)
		}
	}
}
