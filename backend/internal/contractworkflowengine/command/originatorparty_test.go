package command

import (
	"testing"

	"digital-contracting-service/internal/base/datatype"

	"github.com/stretchr/testify/require"
)

// A party is identified by two keys: the did:web of the instance it acts on,
// and the organization within that instance. bindOriginatorParty must write
// both onto ONE node.
//
// When they land on different nodes the two halves of the model never meet:
// mergePartyNodes folds on "@id", so a legal-name node under a #party-N IRI
// can never fold into the DID node, and the read-ACL reads only
// dcs:legalName, so it cannot see a DID-keyed node at all. The originator
// then holds a contract naming it as a party that it is not authorized to
// read back.

func TestBindOriginatorPartyWritesBothIdentityKeysOntoOneNode(t *testing.T) {
	document, err := datatype.NewJSON(map[string]any{
		"@id":         "urn:uuid:contract-1",
		"dcs:parties": []any{},
	})
	require.NoError(t, err)

	bound, err := bindOriginatorParty(&document, originDID, "provider", "Acme Corp")
	require.NoError(t, err)

	node := partyNodesOf(t, *bound)[originDID]
	require.NotNil(t, node, "the originator must be a party node keyed by its did:web")
	require.Equal(t, "provider", node["dcs:role"])
	require.Equal(t, "Acme Corp", node["dcs:legalName"],
		"the same node must carry the organization, or the read-ACL cannot see the originator")
}

// The role placeholder path is the one templates take: materializeRuleParties
// seeds "#party-<role>" nodes from the ODRL rules, and binding rewrites that
// IRI to the origin DID. The organization has to reach the rewritten node too,
// not only the appended-node path.
func TestBindOriginatorPartyNamesTheOrganizationOnARewrittenPlaceholder(t *testing.T) {
	document, err := datatype.NewJSON(map[string]any{
		"@id": "urn:uuid:contract-1",
		"dcs:parties": []any{
			map[string]any{
				"@id":      "urn:uuid:contract-1#party-provider",
				"@type":    "dcs:CompanyParty",
				"dcs:role": "provider",
			},
		},
	})
	require.NoError(t, err)

	bound, err := bindOriginatorParty(&document, originDID, "provider", "Acme Corp")
	require.NoError(t, err)

	nodes := partyNodesOf(t, *bound)
	require.NotContains(t, nodes, "urn:uuid:contract-1#party-provider",
		"the placeholder IRI must be rewritten to the origin DID")
	require.Equal(t, "Acme Corp", nodes[originDID]["dcs:legalName"])
	require.Equal(t, "provider", nodes[originDID]["dcs:role"])
}

// A legal name already recorded for the party wins: attachContractParties may
// have named the organization from the caller's own parties list, and binding
// must not overwrite what the document already asserts.
func TestBindOriginatorPartyKeepsALegalNameTheDocumentAlreadyCarries(t *testing.T) {
	document, err := datatype.NewJSON(map[string]any{
		"@id": "urn:uuid:contract-1",
		"dcs:parties": []any{
			map[string]any{
				"@id":           "urn:uuid:contract-1#party-provider",
				"@type":         "dcs:CompanyParty",
				"dcs:role":      "provider",
				"dcs:legalName": "Acme Corp GmbH",
			},
		},
	})
	require.NoError(t, err)

	bound, err := bindOriginatorParty(&document, originDID, "provider", "Acme Corp")
	require.NoError(t, err)

	require.Equal(t, "Acme Corp GmbH", partyNodesOf(t, *bound)[originDID]["dcs:legalName"])
}
