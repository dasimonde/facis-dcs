package compiler

import "testing"

// TestConvertedContractPassesSHACL mirrors the exact document the DCS
// produces when a contract is instantiated from a template
// (ConvertTemplateDataToContractData): @type dcs:Contract, the metadata
// node retyped to dcs:ContractMetadata, derivedFromTemplate provenance and
// materialized dcs:CompanyParty nodes. The /render gate must accept it —
// dcs:Contract is a SHACL target since the contract-field model landed.
func TestConvertedContractPassesSHACL(t *testing.T) {
	loadSHACLForTest(t)
	converted := []byte(`{
		"@context": {"dcs": "https://w3id.org/facis/dcs/ontology/v1#", "odrl": "http://www.w3.org/ns/odrl/2/"},
		"@id": "http://dcs-a.localhost:18080/digital-contracting-service/api/contract/0565fefd-800f-4cb7-8394-b52a70dd4f00",
		"@type": "dcs:Contract",
		"dcs:metadata": {
			"@id": "http://dcs-a.localhost:18080/digital-contracting-service/api/contract/0565fefd-800f-4cb7-8394-b52a70dd4f00#metadata",
			"@type": "dcs:ContractMetadata",
			"dcs:title": "BDD Contract Source Template"
		},
		"derivedFromTemplate": {
			"@id": "http://dcs-a.localhost:18080/digital-contracting-service/api/template/5eae1cd1-4a93-488c-96f2-34a3f833300b",
			"version": 1
		},
		"dcs:parties": [
			{"@id": "http://dcs-a.localhost:18080/digital-contracting-service/api/contract/0565fefd-800f-4cb7-8394-b52a70dd4f00#party-provider", "@type": "dcs:CompanyParty", "dcs:role": "provider"}
		],
		"dcs:contractData": [],
		"dcs:policies": [],
		"dcs:documentStructure": {
			"@id": "http://dcs-a.localhost:18080/digital-contracting-service/api/contract/0565fefd-800f-4cb7-8394-b52a70dd4f00#document-structure",
			"@type": "dcs:DocumentStructure",
			"dcs:blocks": {"@list": [
				{"@id": "http://dcs-a.localhost:18080/digital-contracting-service/api/contract/0565fefd-800f-4cb7-8394-b52a70dd4f00#block-clause-1", "@type": "dcs:Clause", "dcs:content": {"@list": ["Confidentiality clause"]}}
			]},
			"dcs:layout": {"@list": [
				{"@id": "http://dcs-a.localhost:18080/digital-contracting-service/api/contract/0565fefd-800f-4cb7-8394-b52a70dd4f00#block-root", "@type": "dcs:LayoutNode", "dcs:isRoot": true,
				 "dcs:children": {"@list": [{"@id": "http://dcs-a.localhost:18080/digital-contracting-service/api/contract/0565fefd-800f-4cb7-8394-b52a70dd4f00#block-clause-1"}]}}
			]}
		}
	}`)
	if err := ValidatePayloadSHACL(converted); err != nil {
		t.Fatalf("converted contract must pass the render SHACL gate, got: %v", err)
	}
	canonical, err := CanonicalizePayload(converted)
	if err != nil {
		t.Fatalf("canonicalize: %v", err)
	}
	if err := ValidatePayloadSHACL(canonical); err != nil {
		t.Fatalf("canonical form of a converted contract must pass SHACL, got: %v", err)
	}
}
