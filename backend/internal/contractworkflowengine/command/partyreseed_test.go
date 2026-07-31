package command

import (
	"context"
	"encoding/json"
	"testing"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/contractworkflowengine/db"

	"github.com/stretchr/testify/require"
)

const (
	originDID       = "did:web:origin.example"
	counterpartyDID = "did:web:counterparty.example"
)

// partyNodesOf decodes dcs:parties as a map from the node's IRI to the node.
func partyNodesOf(t *testing.T, raw datatype.JSON) map[string]map[string]any {
	t.Helper()
	var doc map[string]any
	require.NoError(t, json.Unmarshal(raw, &doc))
	nodes, _ := doc["dcs:parties"].([]any)
	out := map[string]map[string]any{}
	for _, rawNode := range nodes {
		node, ok := rawNode.(map[string]any)
		require.True(t, ok)
		iri, _ := node["@id"].(string)
		require.NotContains(t, out, iri, "duplicate party node for %s", iri)
		out[iri] = node
	}
	return out
}

// documentWithoutParties is what a client posts back after an edit: it rebuilds
// the document from its own editor state, and everything it does not model —
// the party nodes above all — is simply absent from what it sends.
func documentWithoutParties(t *testing.T, signatureFields []any) datatype.JSON {
	t.Helper()
	doc := map[string]any{
		"@id":   "did:web:facis.example:contract:1",
		"@type": "dcs:Contract",
		"dcs:documentStructure": map[string]any{
			"@type": "dcs:DocumentStructure",
		},
	}
	if signatureFields != nil {
		doc["dcs:signatureFields"] = signatureFields
	}
	encoded, err := datatype.NewJSON(doc)
	require.NoError(t, err)
	return encoded
}

// The defect: a client rebuild strips dcs:parties but keeps the signature
// fields, and nothing ever put the nodes back — so the fields went on naming
// parties the document no longer declared and every signature attributed to
// nobody. The seed has to restore the nodes for exactly the fields that remain.
func TestSeedRestoresPartyNodesStrippedFromADocumentThatKeptItsSignatureFields(t *testing.T) {
	parties := []string{originDID, counterpartyDID}
	genesis, changed, err := SeedPartiesAndSignatureFields(documentWithoutParties(t, nil), parties)
	require.NoError(t, err)
	require.True(t, changed)
	fields := signatureFieldsOf(t, genesis)
	require.Len(t, fields, 2)

	edited := documentWithoutParties(t, signatureFieldNodes(t, genesis))
	require.Empty(t, partyNodesOf(t, edited), "the client rebuild must be the stripped document")

	reseeded, changed, err := SeedPartiesAndSignatureFields(edited, parties)
	require.NoError(t, err)
	require.True(t, changed)

	nodes := partyNodesOf(t, reseeded)
	require.Len(t, nodes, 2)
	for _, did := range parties {
		require.Contains(t, nodes, did)
		require.Equal(t, "dcs:CompanyParty", nodes[did]["@type"])
	}
	// Every signature field still names a party the document declares.
	for signatory := range signatureFieldsOf(t, reseeded) {
		require.Contains(t, nodes, signatory)
	}
	require.Equal(t, fields, signatureFieldsOf(t, reseeded), "the surviving signature fields must be left alone")
}

// Re-seeding runs on documents that never lost anything, so it must add nothing
// the second time round and report no change — a spurious change would rewrite
// contract_data on every negotiation and re-ship the contract for nothing.
func TestSeedIsIdempotentAndAddsNoDuplicatePartyNodes(t *testing.T) {
	parties := []string{originDID, counterpartyDID}
	first, changed, err := SeedPartiesAndSignatureFields(documentWithoutParties(t, nil), parties)
	require.NoError(t, err)
	require.True(t, changed)

	second, changed, err := SeedPartiesAndSignatureFields(first, parties)
	require.NoError(t, err)
	require.False(t, changed)
	require.JSONEq(t, string(first), string(second))
	require.Len(t, partyNodesOf(t, second), 2)
}

// A party node is where an applied signature is recorded (recordSignatory
// stamps dcs:hasSignatory and the Power of Attorney behind it onto the node).
// Re-seeding must never overwrite a node it finds, or a later save would erase
// the evidence of a signature that was already applied.
func TestReSeedPreservesAttributionAlreadyRecordedOnAPartyNode(t *testing.T) {
	var doc map[string]any
	seeded, _, err := SeedPartiesAndSignatureFields(documentWithoutParties(t, nil), []string{originDID, counterpartyDID})
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(seeded, &doc))

	nodes, _ := doc["dcs:parties"].([]any)
	for _, rawNode := range nodes {
		node := rawNode.(map[string]any)
		if node["@id"] == originDID {
			node["dcs:hasSignatory"] = map[string]any{"@id": "did:jwk:signer"}
			node["dcs:hasPowerOfAttorney"] = map[string]any{"@id": originDID}
		}
	}
	signed, err := datatype.NewJSON(doc)
	require.NoError(t, err)

	reseeded, changed, err := SeedPartiesAndSignatureFields(signed, []string{originDID, counterpartyDID})
	require.NoError(t, err)
	require.False(t, changed)

	origin := partyNodesOf(t, reseeded)[originDID]
	require.Equal(t, map[string]any{"@id": "did:jwk:signer"}, origin["dcs:hasSignatory"])
	require.Equal(t, map[string]any{"@id": originDID}, origin["dcs:hasPowerOfAttorney"])
}

// The workflow's party set is what must be present, but the document is not
// pruned to match it: a party node dropped because the counterparty was
// re-assigned could be one carrying an applied signature.
func TestSeedAddsAReassignedCounterpartyWithoutDroppingTheOldOne(t *testing.T) {
	seeded, _, err := SeedPartiesAndSignatureFields(documentWithoutParties(t, nil), []string{originDID, counterpartyDID})
	require.NoError(t, err)

	reassigned, changed, err := SeedContractParties(seeded, []string{originDID, "did:web:other.example"})
	require.NoError(t, err)
	require.True(t, changed)

	nodes := partyNodesOf(t, reassigned)
	require.Contains(t, nodes, "did:web:other.example")
	require.Contains(t, nodes, counterpartyDID)
}

// The negotiate and submit path: both replace the document with one a client
// assembled and then re-seed what was stored, so a stripped document is put
// back together and persisted.
func TestReseedPersistsRestoredPartiesForTheStoredDocument(t *testing.T) {
	stripped := documentWithoutParties(t, nil)
	repo := &submitContractRepoFake{stored: &db.Contract{
		DID:          "did:web:facis.example:contract:1",
		ContractData: &stripped,
		Responsible:  &db.Responsible{Creator: originDID, Counterparty: counterpartyDID},
	}}

	require.NoError(t, reseedPartiesAndSignatureFields(context.Background(), nil, repo, "did:web:facis.example:contract:1"))

	require.NotNil(t, repo.updated, "the restored document was not persisted")
	require.NotNil(t, repo.updated.ContractData)
	nodes := partyNodesOf(t, *repo.updated.ContractData)
	require.Contains(t, nodes, originDID)
	require.Contains(t, nodes, counterpartyDID)
	require.Len(t, signatureFieldsOf(t, *repo.updated.ContractData), 2)
}

// Nothing to restore means nothing is written: re-seeding must not bump the
// stored document on every negotiation round.
func TestReseedWritesNothingWhenTheDocumentStillDeclaresItsParties(t *testing.T) {
	complete, _, err := SeedPartiesAndSignatureFields(documentWithoutParties(t, nil), []string{originDID, counterpartyDID})
	require.NoError(t, err)
	repo := &submitContractRepoFake{stored: &db.Contract{
		DID:          "did:web:facis.example:contract:1",
		ContractData: &complete,
		Responsible:  &db.Responsible{Creator: originDID, Counterparty: counterpartyDID},
	}}

	require.NoError(t, reseedPartiesAndSignatureFields(context.Background(), nil, repo, "did:web:facis.example:contract:1"))

	require.Nil(t, repo.updated, "an unchanged document must not be rewritten")
}

// signatureFieldNodes returns the raw dcs:signatureFields list of a document.
func signatureFieldNodes(t *testing.T, raw datatype.JSON) []any {
	t.Helper()
	var doc map[string]any
	require.NoError(t, json.Unmarshal(raw, &doc))
	fields, _ := doc["dcs:signatureFields"].([]any)
	require.NotEmpty(t, fields)
	return fields
}
