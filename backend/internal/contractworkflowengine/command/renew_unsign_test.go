package command

import (
	"encoding/json"
	"testing"

	"digital-contracting-service/internal/base/datatype"

	"github.com/stretchr/testify/require"
)

// signedTermDocument is what a renewal copies: a contract that ran its full
// term. Its policy set was sealed into the odrl:Agreement the signatures bind,
// both party nodes carry the ODRL role the seal assigned, and the origin's node
// carries the signatory who signed for it and the Power of Attorney they signed
// under. Alongside them stands the read-authorization node attachContractParties
// records, whose dcs:legalName gates read access.
func signedTermDocument(t *testing.T) *datatype.JSON {
	t.Helper()
	doc := map[string]any{
		"@id":   "did:web:facis.example:contract:1",
		"@type": "dcs:Contract",
		"dcs:documentStructure": map[string]any{
			"@id":   "did:web:facis.example:contract:1#document-structure",
			"@type": "dcs:DocumentStructure",
			"dcs:blocks": map[string]any{"@list": []any{
				map[string]any{"@type": "dcs:Clause", "dcs:text": "the agreed term"},
			}},
		},
		"dcs:policies": map[string]any{
			"@id":          "urn:uuid:policy-1",
			"@type":        "odrl:Agreement",
			"odrl:profile": map[string]any{"@id": "https://w3id.org/facis/dcs/profile"},
		},
		"dcs:parties": []any{
			map[string]any{
				"@id":                     originDID,
				"@type":                   "dcs:CompanyParty",
				"dcs:role":                "supplier",
				"odrl:function":           map[string]any{"@id": "odrl:contractingParty"},
				"dcs:hasSignatory":        map[string]any{"@id": "did:jwk:alice"},
				"dcs:hasPowerOfAttorney":  map[string]any{"@id": originDID},
				"dcs:someUnrelatedDetail": "kept",
			},
			map[string]any{
				"@id":                    counterpartyDID,
				"@type":                  "dcs:CompanyParty",
				"odrl:function":          map[string]any{"@id": "odrl:contractedParty"},
				"dcs:hasSignatory":       map[string]any{"@id": "did:jwk:bob"},
				"dcs:hasPowerOfAttorney": map[string]any{"@id": counterpartyDID},
			},
			map[string]any{
				"@id":           "did:web:facis.example:contract:1#party-0",
				"@type":         "dcs:CompanyParty",
				"dcs:legalName": "Origin GmbH",
			},
		},
		"dcs:signatureFields": []any{
			map[string]any{
				"@id":               "did:web:facis.example:contract:1#signature-field-a",
				"@type":             "dcs:SignatureField",
				"dcs:signatoryName": originDID,
			},
			map[string]any{
				"@id":               "did:web:facis.example:contract:1#signature-field-b",
				"@type":             "dcs:SignatureField",
				"dcs:signatoryName": counterpartyDID,
			},
		},
	}
	encoded, err := datatype.NewJSON(doc)
	require.NoError(t, err)
	return &encoded
}

// The defect: a renewal copied the previous term's document verbatim, so a
// brand-new draft nobody had signed asserted in its machine-readable contract
// that named people signed it, under a named Power of Attorney. That assertion
// is what gets embedded in the PDF, shipped to the peer, and read back by the
// Power-of-Attorney compliance findings.
func TestRenewalCarriesNoSignatureAttributionFromThePreviousTerm(t *testing.T) {
	renewal, err := unsignRenewedDocument(signedTermDocument(t))
	require.NoError(t, err)

	nodes := partyNodesOf(t, *renewal)
	require.Len(t, nodes, 3, "no party node may be dropped")
	for iri, node := range nodes {
		require.NotContains(t, node, "dcs:hasSignatory", "party %s still claims a signatory", iri)
		require.NotContains(t, node, "dcs:hasPowerOfAttorney", "party %s still claims a Power of Attorney", iri)
		require.NotContains(t, node, "odrl:function", "party %s still claims an accepted ODRL role", iri)
	}
}

// The read-authorization nodes (attachContractParties' #party-N and their
// dcs:legalName) gate read access in query/contract/querybyid.go, and
// CreateCmd.Parties is not persisted — dropping one could never be undone.
// Everything else the party node says about the party is equally not a
// signature and stays.
func TestRenewalKeepsThePartyNodesThatGateReadAccess(t *testing.T) {
	renewal, err := unsignRenewedDocument(signedTermDocument(t))
	require.NoError(t, err)

	nodes := partyNodesOf(t, *renewal)
	readACL := nodes["did:web:facis.example:contract:1#party-0"]
	require.NotNil(t, readACL, "the read-authorization node was removed")
	require.Equal(t, "Origin GmbH", readACL["dcs:legalName"])

	require.Equal(t, "supplier", nodes[originDID]["dcs:role"])
	require.Equal(t, "kept", nodes[originDID]["dcs:someUnrelatedDetail"])
	require.Equal(t, "dcs:CompanyParty", nodes[counterpartyDID]["@type"])
}

// A renewal offers the same terms afresh. The seal is the first signature's
// acceptance act (sealAgreementForSigning retypes the offered policy set into
// the odrl:Agreement those signatures bind), so a draft nobody has accepted
// must be an odrl:Offer again.
func TestRenewalIsAnOfferAgainAndNotASealedAgreement(t *testing.T) {
	renewal, err := unsignRenewedDocument(signedTermDocument(t))
	require.NoError(t, err)

	var doc map[string]any
	require.NoError(t, json.Unmarshal(*renewal, &doc))
	policies, ok := doc["dcs:policies"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "odrl:Offer", policies["@type"])
	require.Equal(t, "urn:uuid:policy-1", policies["@id"], "the policy set itself must survive")
	require.Contains(t, policies, "odrl:profile")
}

// Clearing the attribution must not cost the renewal the terms it exists to
// carry forward (DCS-FR-CWE-22), nor the signature fields, which say where the
// new term is to be signed rather than that it was.
func TestRenewalStillInheritsTheContractsTerms(t *testing.T) {
	renewal, err := unsignRenewedDocument(signedTermDocument(t))
	require.NoError(t, err)

	var doc map[string]any
	require.NoError(t, json.Unmarshal(*renewal, &doc))
	structure, ok := doc["dcs:documentStructure"].(map[string]any)
	require.True(t, ok)
	blocks, ok := structure["dcs:blocks"].(map[string]any)
	require.True(t, ok, "the ordered block list must survive as an explicit @list")
	require.Len(t, blocks["@list"], 1)

	fields := signatureFieldsOf(t, *renewal)
	require.Len(t, fields, 2)
	require.Contains(t, fields, originDID)
	require.Contains(t, fields, counterpartyDID)
}

// Renewing a contract that was never signed must behave exactly as it did
// before: there is nothing to clear, so the document passes through untouched.
func TestRenewalOfAnUnsignedContractLeavesTheDocumentUntouched(t *testing.T) {
	seeded, _, err := SeedPartiesAndSignatureFields(documentWithoutParties(t, nil), []string{originDID, counterpartyDID})
	require.NoError(t, err)

	renewal, err := unsignRenewedDocument(&seeded)
	require.NoError(t, err)
	require.JSONEq(t, string(seeded), string(*renewal))
}

// The seeding that establishes the new term runs over the cleared document, and
// it is additive and idempotent — it must not put back what was just removed,
// nor duplicate the parties the copy already declares.
func TestSeedingTheClearedRenewalRestoresNoAttribution(t *testing.T) {
	renewal, err := unsignRenewedDocument(signedTermDocument(t))
	require.NoError(t, err)

	seeded, changed, err := SeedPartiesAndSignatureFields(*renewal, []string{originDID, counterpartyDID})
	require.NoError(t, err)
	require.False(t, changed, "the copy already declares both parties and their fields")

	for iri, node := range partyNodesOf(t, seeded) {
		require.NotContains(t, node, "dcs:hasSignatory", "party %s was re-attributed by the seed", iri)
		require.NotContains(t, node, "dcs:hasPowerOfAttorney", "party %s was re-attributed by the seed", iri)
	}
}
