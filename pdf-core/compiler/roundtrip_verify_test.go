package compiler

import (
	"context"
	"testing"
	"time"
)

// roundtripMatches reproduces exactly what a receiving DCS's /verify/content
// does to a peer-shipped PDF: compile the payload, extract the embedded
// canonical JSON-LD back out, recompile it, and require the page content to
// match. A false result is what makes PostPdf's fatal VerifyContent gate reject
// a legitimately-offered contract.
func roundtripMatches(t *testing.T, payload string) error {
	t.Helper()
	at := time.Date(2026, 6, 4, 0, 0, 0, 0, time.UTC)
	ctx := WithSigner(context.Background(), NewCapturingSigner())

	original, err := CompilePDF(ctx, []byte(payload), at)
	if err != nil {
		t.Fatalf("compile original: %v", err)
	}
	embedded, err := ExtractLatestEmbeddedJSONLD(original)
	if err != nil {
		t.Fatalf("extract embedded JSON-LD: %v", err)
	}
	recompiled, err := CompilePDF(ctx, embedded, at)
	if err != nil {
		t.Fatalf("recompile from embedded: %v", err)
	}
	return MatchPageContent(original, recompiled)
}

// TestRoundtrip_Minimal is the control: a minimal contract round-trips cleanly,
// which is why minimal peer replication (BDD feature:78) verifies fine.
func TestRoundtrip_Minimal(t *testing.T) {
	if err := roundtripMatches(t, referencePayload); err != nil {
		t.Fatalf("minimal payload should round-trip but did not: %v", err)
	}
}

// richPayload mirrors what the two-instance e2e vertical authors and offers: a
// contract with a valued requirement field, an ODRL policy set, and seeded
// signature fields — the shape that fails B's VerifyContent gate in CI.
const richPayload = `{
	"@context": {
		"@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
		"dcs": "https://w3id.org/facis/dcs/ontology/v1#",
		"odrl": "http://www.w3.org/ns/odrl/2/"
	},
	"@id": "urn:doc:rich-ref",
	"@type": "ContractTemplate",
	"metadata": {"@type": "TemplateMetadata", "title": "Rich Contract Vertical"},
	"documentStructure": {
		"@type": "DocumentStructure",
		"layout": [
			{"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:rich-ref#s1", "urn:doc:rich-ref#s2"]},
			{"@type": "LayoutNode", "@id": "urn:doc:rich-ref#s1", "children": ["urn:doc:rich-ref#c1"]},
			{"@type": "LayoutNode", "@id": "urn:doc:rich-ref#s2", "children": ["urn:doc:rich-ref#c2"]}
		],
		"blocks": [
			{"@type": "Section", "@id": "urn:doc:rich-ref#s1", "title": "1. Payment terms"},
			{"@type": "Clause", "@id": "urn:doc:rich-ref#c1", "content": ["The provider invoices the agreed payment amount."]},
			{"@type": "Section", "@id": "urn:doc:rich-ref#s2", "title": "2. Obligations"},
			{"@type": "Clause", "@id": "urn:doc:rich-ref#c2", "content": ["The provider provides the data."]}
		]
	},
	"parameterValue": [
		{"@type": "RequirementField", "@id": "urn:doc:rich-ref#field-amount", "title": "Payment Amount", "parameterValue": "15000"}
	],
	"policies": {
		"@type": "odrl:Set",
		"@id": "urn:doc:rich-ref#policy",
		"odrl:profile": {"@id": "https://w3id.org/facis/dcs/odrl-profile/v1"},
		"odrl:permission": [
			{
				"@type": "odrl:Permission",
				"odrl:action": {"@id": "https://w3id.org/facis/dcs/action/use"},
				"odrl:assignee": {"@id": "urn:doc:rich-ref#party-customer"},
				"odrl:target": {"@id": "urn:doc:rich-ref"},
				"odrl:constraint": {
					"@type": "odrl:Constraint",
					"odrl:leftOperand": {"@id": "urn:doc:rich-ref#field-amount"},
					"odrl:operator": {"@id": "http://www.w3.org/ns/odrl/2/lteq"},
					"odrl:rightOperand": "500"
				}
			}
		]
	},
	"signatureFields": [
		{"@type": "SignatureField", "@id": "urn:doc:rich-ref#did:web:dcs-a", "signatoryName": "did:web:dcs-a.localhost%3A18080"},
		{"@type": "SignatureField", "@id": "urn:doc:rich-ref#did:web:dcs-b", "signatoryName": "did:web:dcs-b.localhost%3A18080"}
	]
}`

// TestRoundtrip_Rich reproduces the stage-5 failure: if the rich contract does
// NOT round-trip, B's fatal VerifyContent rejects it and it never lands.
func TestRoundtrip_Rich(t *testing.T) {
	if err := roundtripMatches(t, richPayload); err != nil {
		t.Fatalf("RICH payload does not round-trip (this is the stage-5 VerifyContent rejection): %v", err)
	}
}
