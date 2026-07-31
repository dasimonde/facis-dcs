package command

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/validation"
	cwecommand "digital-contracting-service/internal/contractworkflowengine/command"
	cwedb "digital-contracting-service/internal/contractworkflowengine/db"
	"digital-contracting-service/internal/contractworkflowengine/query/contracttemplate"
	db "digital-contracting-service/internal/signingmanagement/db"
)

const (
	instanceA  = "did:web:dcs-a.localhost%3A18080"
	instanceB  = "did:web:dcs-b.localhost%3A18080"
	signerA    = "did:jwk:eyJrdHkiOiJFQyJ9"
	contractID = "6252fce0-d5bc-45ea-b5f7-683dd53dc17d"
)

// bddContractTemplate is the template every two-instance scenario is built from
// (steps/support/services/template_service.py canonical_document_data): a bare
// document structure, no ODRL policies and therefore no role-derived party
// placeholders, and no legal names — a contract offered to a peer names neither
// organization by name, only both instances by DID.
func bddContractTemplate(t *testing.T) *datatype.JSON {
	t.Helper()
	raw, err := datatype.NewJSON(map[string]any{
		"@context": map[string]any{"dcs": "https://w3id.org/facis/dcs/ontology/v1#"},
		"@type":    "dcs:ContractTemplate",
		"dcs:metadata": map[string]any{
			"@type":     "dcs:TemplateMetadata",
			"dcs:title": "BDD Contract Source Template",
		},
		"dcs:documentStructure": map[string]any{
			"@type": "dcs:DocumentStructure",
			"dcs:blocks": map[string]any{"@list": []any{
				map[string]any{
					"@id":         "urn:uuid:block-clause-1",
					"@type":       "dcs:Clause",
					"dcs:content": map[string]any{"@list": []any{"Confidentiality clause"}},
				},
			}},
			"dcs:layout": []any{
				map[string]any{
					"@id":          "urn:uuid:block-root",
					"@type":        "dcs:LayoutNode",
					"dcs:isRoot":   true,
					"dcs:children": map[string]any{"@list": []any{map[string]any{"@id": "urn:uuid:block-clause-1"}}},
				},
			},
		},
	})
	require.NoError(t, err)
	return &raw
}

// genesisContractDocument reproduces what contractworkflowengine/command's
// Creator persists for the two-instance flow: template conversion, persistence
// normalization, then the party and signature-field seeding. The scenario offers
// a contract naming instance B as counterparty and passes neither `parties` nor
// `originator_role`, so attachContractParties and bindOriginatorParty are both
// no-ops — every party node the document ends up with is seeded here.
func genesisContractDocument(t *testing.T, counterparty string) datatype.JSON {
	t.Helper()

	parties := (&cwedb.Responsible{Creator: instanceA, Counterparty: counterparty}).GetParties()

	contractDocument, err := contracttemplate.ConvertTemplateDataToContractData(bddContractTemplate(t), "urn:uuid:bdd-template", 1)
	require.NoError(t, err)
	normalized, err := validation.NormalizeContractDataForPersistence(contractDocument, contractID, false)
	require.NoError(t, err)

	withParties, changed, err := cwecommand.SeedContractParties(*normalized, parties)
	require.NoError(t, err)
	require.True(t, changed, "a contract whose template declares no parties must have them seeded")

	seeded, changed, err := cwecommand.SeedSignatureFields(withParties, parties)
	require.NoError(t, err)
	require.True(t, changed, "a contract whose template declares no signature fields must have them seeded")
	return seeded
}

func partyNodes(t *testing.T, raw datatype.JSON) map[string]map[string]any {
	t.Helper()
	var doc map[string]any
	require.NoError(t, json.Unmarshal(raw, &doc))
	declared, _ := doc["dcs:parties"].([]any)
	nodes := map[string]map[string]any{}
	for _, rawNode := range declared {
		node := rawNode.(map[string]any)
		iri, _ := node["@id"].(string)
		require.NotContains(t, nodes, iri, "one node per party IRI")
		nodes[iri] = node
	}
	return nodes
}

// The originator signs its own seeded signature field on a contract offered to a
// peer. That is the whole two-instance signing flow, and its outcome is what the
// counterparty verifies (ADR-31): the party the credential authorizes must carry
// the signatory that signed for it and the Power of Attorney it signed under.
//
// The document reaching this point carried no party nodes at all — the template
// declares none and the two-instance create passes neither read-authorization
// names nor an originator role — so the attribution had nowhere to land, and the
// peer received a signed contract crediting its signature to nobody.
func TestOriginatorSignatureIsAttributedToItsOwnPartyOnATwoInstanceContract(t *testing.T) {
	responsible := &db.Responsible{Creator: instanceA, Counterparty: instanceB}
	genesis := genesisContractDocument(t, instanceB)

	// The ceremony's field name IS the signing party: the seeded fields are named
	// for the instance DIDs, and the ceremony refuses unless the Power of Attorney
	// authorizes exactly that party (ceremony.go).
	sealed, err := sealAgreementForSigning(genesis, responsible, signerA)
	require.NoError(t, err)
	attributed, err := recordSignatory(sealed, responsible, signerA, instanceA, instanceA)
	require.NoError(t, err)

	nodes := partyNodes(t, attributed)
	require.Contains(t, nodes, instanceA, "the originating instance is a party of its own contract")
	require.Contains(t, nodes, instanceB, "the counterparty the contract is offered to is a party of it")

	require.Equal(t, map[string]any{"@id": signerA}, nodes[instanceA]["dcs:hasSignatory"],
		"the party that signed must record who signed for it")
	require.Equal(t, map[string]any{"@id": instanceA}, nodes[instanceA]["dcs:hasPowerOfAttorney"],
		"the party that signed must record the authority it signed under")

	require.NotContains(t, nodes[instanceB], "dcs:hasSignatory",
		"the counterparty has not signed on this instance")
}

// Every seeded signature field must name a party the document declares: the
// ceremony binds a Power of Attorney to the field's name, and the signature is
// attributed to the party node carrying that same IRI. A field naming a party
// that does not exist silently loses both.
func TestEverySeededSignatureFieldNamesADeclaredParty(t *testing.T) {
	genesis := genesisContractDocument(t, instanceB)

	var doc map[string]any
	require.NoError(t, json.Unmarshal(genesis, &doc))
	fields, _ := doc["dcs:signatureFields"].([]any)
	require.Len(t, fields, 2, "one signature field per party")

	nodes := partyNodes(t, genesis)
	for _, rawField := range fields {
		signatory, _ := rawField.(map[string]any)["dcs:signatoryName"].(string)
		require.Contains(t, nodes, signatory, "signature field %q names no party node", signatory)
	}
}

// A contract whose ODRL rules carry a role placeholder still binds it to the
// counterparty when the offer is accepted — and the counterparty is now already
// declared, so the binding must collapse the two nodes rather than leave a
// second one shadowing the first.
func TestBindingARolePlaceholderCollapsesOntoTheDeclaredCounterparty(t *testing.T) {
	responsible := &db.Responsible{Creator: instanceA, Counterparty: instanceB}
	raw, err := datatype.NewJSON(map[string]any{
		"@id":          "urn:contract:1",
		"dcs:policies": map[string]any{"@type": "odrl:Offer"},
		"dcs:parties": []any{
			map[string]any{"@id": "urn:contract:1#party-customer", "@type": "dcs:CompanyParty", "dcs:role": "customer"},
			map[string]any{"@id": instanceA, "@type": "dcs:CompanyParty"},
			map[string]any{"@id": instanceB, "@type": "dcs:CompanyParty"},
		},
	})
	require.NoError(t, err)

	sealed, err := sealAgreementForSigning(raw, responsible, signerA)
	require.NoError(t, err)
	attributed, err := recordSignatory(sealed, responsible, signerA, instanceA, instanceA)
	require.NoError(t, err)

	nodes := partyNodes(t, attributed)
	require.Len(t, nodes, 2, "the bound placeholder and the declared counterparty are one party")
	require.Equal(t, "customer", nodes[instanceB]["dcs:role"], "the role the placeholder carried survives the binding")
	require.Equal(t, map[string]any{"@id": "odrl:contractedParty"}, nodes[instanceB]["odrl:function"])
	require.Equal(t, map[string]any{"@id": signerA}, nodes[instanceA]["dcs:hasSignatory"])
}

// The single-instance flow signs its own field with no counterparty at all: the
// origin is the only party, and its signature is attributed to it rather than
// falling back to the signer's own key identity.
func TestSingleInstanceSignatureIsAttributedToTheOriginParty(t *testing.T) {
	responsible := &db.Responsible{Creator: instanceA}
	genesis := genesisContractDocument(t, "")

	sealed, err := sealAgreementForSigning(genesis, responsible, signerA)
	require.NoError(t, err)
	attributed, err := recordSignatory(sealed, responsible, signerA, instanceA, instanceA)
	require.NoError(t, err)

	nodes := partyNodes(t, attributed)
	require.Len(t, nodes, 1, "a contract with no counterparty has exactly one party")
	require.Equal(t, map[string]any{"@id": signerA}, nodes[instanceA]["dcs:hasSignatory"])
	require.Equal(t, map[string]any{"@id": instanceA}, nodes[instanceA]["dcs:hasPowerOfAttorney"])
}

// Compliance re-reads the attribution the signing flow wrote (FR-SM-04/-26): a
// party authorized for itself raises nothing, and the same document with the
// authority pointing elsewhere raises the finding the viewer exists for.
func TestComplianceReadsTheAttributionTheSigningFlowWrites(t *testing.T) {
	responsible := &db.Responsible{Creator: instanceA, Counterparty: instanceB}
	genesis := genesisContractDocument(t, instanceB)

	sealed, err := sealAgreementForSigning(genesis, responsible, signerA)
	require.NoError(t, err)

	attributed, err := recordSignatory(sealed, responsible, signerA, instanceA, instanceA)
	require.NoError(t, err)
	findings, judged := poaComplianceFindings(attributed)
	require.Empty(t, findings)
	require.True(t, judged[instanceA], "the viewer must report having judged the party the document attributes")

	misauthorized, err := recordSignatory(sealed, responsible, signerA, "did:web:some-other-org.example", instanceA)
	require.NoError(t, err)
	findings, _ = poaComplianceFindings(misauthorized)
	require.Len(t, findings, 1)
}
