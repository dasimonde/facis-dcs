package jades

import (
	"testing"
	"time"
)

func TestContractDocumentDigestIsIndependentOfSerialization(t *testing.T) {
	// Both instances hold the same document but store it through their own
	// jsonb column, which reorders keys and rewrites whitespace and number
	// forms. The digest is the cross-instance version identity, so it must not
	// see any of that.
	forms := []string{
		`{"a":{"nested":true},"b":1.10,"c":"x"}`,
		"  {\n \"c\": \"x\",\n \"b\": 1.1,\n \"a\": {\"nested\": true}\n}",
		`{"b":1.1,"c":"x","a":{"nested":true}}`,
	}
	first, err := ContractDocumentDigest([]byte(forms[0]))
	if err != nil {
		t.Fatal(err)
	}
	for _, form := range forms[1:] {
		digest, err := ContractDocumentDigest([]byte(form))
		if err != nil {
			t.Fatal(err)
		}
		if digest != first {
			t.Fatalf("the same document serialized differently digests to %s and %s", first, digest)
		}
	}
}

func TestContractDocumentDigestChangesWithTheDocument(t *testing.T) {
	before, err := ContractDocumentDigest([]byte(`{"dcs:clause":"the agreed terms"}`))
	if err != nil {
		t.Fatal(err)
	}
	after, err := ContractDocumentDigest([]byte(`{"dcs:clause":"the amended terms"}`))
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("a redline must change the document digest, or a settlement of the old text would authorize the new one")
	}
}

func TestBuildSettlementPayloadIsReproducible(t *testing.T) {
	settlement := Settlement{
		ContractDID:     "did:web:dcs-a.localhost:contract:42",
		ContractVersion: 3,
		DocumentDigest:  "sha256:9f2b",
		SettledBy:       "did:web:dcs-b.localhost",
		SettledWith:     "did:web:dcs-a.localhost",
		SettledAt:       time.Date(2026, 7, 31, 9, 14, 22, 417000000, time.UTC),
	}
	payload, err := BuildSettlementPayload(settlement)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"@context":{"dcs":"https://w3id.org/facis/dcs/ontology/v1#"},` +
		`"@type":"dcs:ContractSettlement",` +
		`"dcs:contractDid":"did:web:dcs-a.localhost:contract:42",` +
		`"dcs:contractDocumentDigest":"sha256:9f2b",` +
		`"dcs:contractVersion":3,` +
		`"dcs:settledAt":"2026-07-31T09:14:22.417Z",` +
		`"dcs:settledBy":"did:web:dcs-b.localhost",` +
		`"dcs:settledWith":"did:web:dcs-a.localhost"}`
	if string(payload) != want {
		t.Fatalf("settlement payload is\n%s\nwant\n%s", payload, want)
	}

	// The receiver re-derives these bytes from the fields it read out of the
	// verified payload, so a timestamp given in another zone must land on the
	// same canonical form rather than on a second, differently written one.
	elsewhere := settlement
	elsewhere.SettledAt = settlement.SettledAt.In(time.FixedZone("CEST", 2*60*60))
	again, err := BuildSettlementPayload(elsewhere)
	if err != nil {
		t.Fatal(err)
	}
	if string(again) != want {
		t.Fatalf("the same instant in another zone canonicalizes to\n%s", again)
	}
}
