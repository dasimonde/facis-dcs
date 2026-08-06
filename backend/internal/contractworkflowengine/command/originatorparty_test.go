package command

import (
	"encoding/json"
	"testing"

	"digital-contracting-service/internal/base/datatype"

	"github.com/stretchr/testify/require"
)

func TestValidateOriginatorRoleRequiresExactlyTwoMaterializedTemplateRoles(t *testing.T) {
	supplierRole := "https://w3id.org/facis/dcs/taxonomy/v1#role-supplier"
	clientRole := "https://w3id.org/facis/dcs/taxonomy/v1#role-client"
	document, err := datatype.NewJSON(map[string]any{
		"dcs:parties": []any{
			map[string]any{"@id": "urn:uuid:contract-1#party-encoded-a", "dcs:role": supplierRole},
			map[string]any{"@id": "urn:uuid:contract-1#party-encoded-b", "dcs:role": clientRole},
		},
	})
	require.NoError(t, err)
	require.NoError(t, validateOriginatorRole(&document, clientRole))
	require.ErrorIs(t, validateOriginatorRole(&document, "client"), ErrInvalidOriginatorRole)
	require.ErrorIs(t, validateOriginatorRole(&document, "unknown"), ErrInvalidOriginatorRole)

	oneRole, err := datatype.NewJSON(map[string]any{
		"dcs:parties": []any{map[string]any{"@id": "urn:uuid:contract-1#party-encoded", "dcs:role": supplierRole}},
	})
	require.NoError(t, err)
	require.ErrorIs(t, validateOriginatorRole(&oneRole, supplierRole), ErrInvalidOriginatorRole)

	shortRoles, err := datatype.NewJSON(map[string]any{
		"dcs:parties": []any{
			map[string]any{"@id": "urn:uuid:contract-1#party-provider", "dcs:role": "provider"},
			map[string]any{"@id": "urn:uuid:contract-1#party-customer", "dcs:role": "customer"},
		},
	})
	require.NoError(t, err)
	require.ErrorIs(t, validateOriginatorRole(&shortRoles, "provider"), ErrInvalidOriginatorRole)
}

func TestBindOriginatorPartyRewritesOnlyTheSelectedRoleAcrossRuleBuckets(t *testing.T) {
	providerRole := "https://w3id.org/facis/dcs/taxonomy/v1#role-provider"
	customerRole := "https://w3id.org/facis/dcs/taxonomy/v1#role-customer"
	providerParty := "urn:uuid:contract-1#party-https%3A%2F%2Fw3id.org%2Ffacis%2Fdcs%2Ftaxonomy%2Fv1%23role-provider"
	customerParty := "urn:uuid:contract-1#party-https%3A%2F%2Fw3id.org%2Ffacis%2Fdcs%2Ftaxonomy%2Fv1%23role-customer"
	rule := func(assigner, assignee string) map[string]any {
		return map[string]any{
			"odrl:assigner": map[string]any{"@id": assigner},
			"odrl:assignee": map[string]any{"@id": assignee},
		}
	}
	doc := map[string]any{
		"@id": "urn:uuid:contract-1",
		"dcs:parties": []any{
			map[string]any{"@id": providerParty, "@type": "dcs:CompanyParty", "dcs:role": providerRole},
			map[string]any{"@id": customerParty, "@type": "dcs:CompanyParty", "dcs:role": customerRole},
		},
		"dcs:policies": map[string]any{
			"odrl:permission":  []any{rule(providerParty, customerParty)},
			"odrl:obligation":  []any{rule(customerParty, providerParty)},
			"odrl:prohibition": []any{rule(providerParty, customerParty), rule(customerParty, providerParty)},
		},
	}
	selectedReferences := countJSONLDReferences(doc["dcs:policies"], providerParty)
	counterpartReferences := countJSONLDReferences(doc["dcs:policies"], customerParty)
	require.Positive(t, selectedReferences)
	require.Positive(t, counterpartReferences)

	document, err := datatype.NewJSON(doc)
	require.NoError(t, err)
	bound, err := bindOriginatorParty(&document, originDID, providerRole, "Provider GmbH")
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(*bound, &result))
	require.Zero(t, countJSONLDReferences(result["dcs:policies"], providerParty))
	require.Equal(t, selectedReferences, countJSONLDReferences(result["dcs:policies"], originDID))
	require.Equal(t, counterpartReferences, countJSONLDReferences(result["dcs:policies"], customerParty))
	nodes := partyNodesOf(t, *bound)
	require.Equal(t, providerRole, nodes[originDID]["dcs:role"])
	require.Equal(t, customerRole, nodes[customerParty]["dcs:role"])
}

func countJSONLDReferences(value any, iri string) int {
	switch current := value.(type) {
	case map[string]any:
		count := 0
		if current["@id"] == iri {
			count++
		}
		for _, nested := range current {
			count += countJSONLDReferences(nested, iri)
		}
		return count
	case []any:
		count := 0
		for _, nested := range current {
			count += countJSONLDReferences(nested, iri)
		}
		return count
	default:
		return 0
	}
}

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

	role := "https://w3id.org/facis/dcs/taxonomy/v1#role-provider"
	bound, err := bindOriginatorParty(&document, originDID, role, "Acme Corp")
	require.NoError(t, err)

	node := partyNodesOf(t, *bound)[originDID]
	require.NotNil(t, node, "the originator must be a party node keyed by its did:web")
	require.Equal(t, role, node["dcs:role"])
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
				"@id":      "urn:uuid:contract-1#party-not-derived-from-role-text",
				"@type":    "dcs:CompanyParty",
				"dcs:role": "https://w3id.org/facis/dcs/taxonomy/v1#role-provider",
			},
		},
	})
	require.NoError(t, err)

	role := "https://w3id.org/facis/dcs/taxonomy/v1#role-provider"
	bound, err := bindOriginatorParty(&document, originDID, role, "Acme Corp")
	require.NoError(t, err)

	nodes := partyNodesOf(t, *bound)
	require.NotContains(t, nodes, "urn:uuid:contract-1#party-not-derived-from-role-text",
		"the placeholder IRI must be rewritten to the origin DID")
	require.Equal(t, "Acme Corp", nodes[originDID]["dcs:legalName"])
	require.Equal(t, role, nodes[originDID]["dcs:role"])
}

// A legal name already recorded for the party wins: attachContractParties may
// have named the organization from the caller's own parties list, and binding
// must not overwrite what the document already asserts.
func TestBindOriginatorPartyKeepsALegalNameTheDocumentAlreadyCarries(t *testing.T) {
	role := "https://w3id.org/facis/dcs/taxonomy/v1#role-provider"
	document, err := datatype.NewJSON(map[string]any{
		"@id": "urn:uuid:contract-1",
		"dcs:parties": []any{
			map[string]any{
				"@id":           "urn:uuid:contract-1#party-encoded-provider",
				"@type":         "dcs:CompanyParty",
				"dcs:role":      role,
				"dcs:legalName": "Acme Corp GmbH",
			},
		},
	})
	require.NoError(t, err)

	bound, err := bindOriginatorParty(&document, originDID, role, "Acme Corp")
	require.NoError(t, err)

	require.Equal(t, "Acme Corp GmbH", partyNodesOf(t, *bound)[originDID]["dcs:legalName"])
}
