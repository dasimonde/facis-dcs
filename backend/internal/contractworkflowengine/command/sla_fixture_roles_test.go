package command

import (
	"encoding/json"
	"os"
	"testing"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/validation"
	contracttemplate "digital-contracting-service/internal/contractworkflowengine/query/contracttemplate"

	"github.com/stretchr/testify/require"
)

// TestSlaFixtureDerivedPartiesCarryVocabularyRoles walks the SLA federation
// fixture through the creation pipeline (derive -> normalize -> read-ACL
// parties -> party/signature-field seeding) and requires that every party
// node carrying a role carries a DCS taxonomy IRI. The facis.sla.basic
// profile's company-party-role-vocabulary rule blocks the offer transition
// for anything else, which is exactly how the SLA federation scenarios fail
// when the fixture's party declarations fall behind the taxonomy.
func TestSlaFixtureDerivedPartiesCarryVocabularyRoles(t *testing.T) {
	rawFile, err := os.ReadFile("../../../../features/fixtures/sla_hosting_template.jsonld")
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(rawFile, &doc))
	if inner, ok := doc["template_data"].(map[string]any); ok {
		doc = inner
	}
	templateRaw, err := datatype.NewJSON(doc)
	require.NoError(t, err)

	normalizedTemplate, err := validation.NormalizeTemplateDataForPersistence(&templateRaw, "did:web:example:template:sla-hosting")
	require.NoError(t, err)

	contractDoc, err := contracttemplate.ConvertTemplateDataToContractData(normalizedTemplate, "did:web:example:template:sla-hosting", 1)
	require.NoError(t, err)
	normalized, err := validation.NormalizeContractDataForPersistence(contractDoc, "11111111-2222-3333-4444-555555555555", false)
	require.NoError(t, err)
	withACL, err := attachContractParties(normalized, []string{"Org B Negotiation GmbH", "Test Organization"})
	require.NoError(t, err)
	seeded, _, err := SeedPartiesAndSignatureFields(*withACL, []string{"did:web:dcs-b.localhost%3A18081", "did:web:dcs-a.localhost%3A18080"})
	require.NoError(t, err)

	var contract map[string]any
	require.NoError(t, json.Unmarshal(seeded, &contract))

	vocabulary := map[string]bool{
		"https://w3id.org/facis/dcs/taxonomy/v1#role-provider": true,
		"https://w3id.org/facis/dcs/taxonomy/v1#role-customer": true,
	}
	parties, ok := contract["dcs:parties"].([]any)
	require.True(t, ok, "the derived contract declares dcs:parties")
	rolesSeen := 0
	for _, p := range parties {
		node := p.(map[string]any)
		role, has := node["dcs:role"].(string)
		if !has {
			continue // role-less read-ACL and per-instance nodes are legitimate placeholders
		}
		rolesSeen++
		require.True(t, vocabulary[role], "party %v carries a non-taxonomy role %q", node["@id"], role)
	}
	require.Equal(t, 2, rolesSeen, "the fixture's two declared template roles survive derivation")
}
