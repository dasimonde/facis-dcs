package template

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPartyAndTemplateFiltersApplyToCountAndPage(t *testing.T) {
	filters := buildSearchFilters(SearchQry{PartyType: "MULTI_PARTY", TemplateType: "COMPONENT"})
	for _, statement := range []string{
		fmt.Sprintf(searchTemplatesCountStatementTemplate, coreFieldTriples(), filters),
		fmt.Sprintf(searchTemplatesStatementTemplate, coreFieldTriples(), filters, 0, 10),
	} {
		require.Contains(t, statement, `FILTER(?party_type = "MULTI_PARTY")`)
		require.Contains(t, statement, `FILTER(?template_type = "COMPONENT")`)
		require.Contains(t, statement, "https://w3id.org/facis/dcs/1#partyType")
	}
	item := mapCatalogueItem(map[string]interface{}{"did": "test", "party_type": "MULTI_PARTY", "template_type": "COMPONENT"})
	require.Equal(t, "MULTI_PARTY", *item.PartyType)
	require.Equal(t, "COMPONENT", *item.TemplateType)
}
