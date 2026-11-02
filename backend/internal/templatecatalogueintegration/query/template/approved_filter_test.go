package template

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// DCS-NFR-BR-09: list, search and retrieve-by-id read only approved catalogue templates.
func TestCatalogueReadsOnlyApprovedTemplates(t *testing.T) {
	for name, statement := range map[string]string{
		"list":   fmt.Sprintf(retrieveTemplatesStatementTemplate, coreFieldTriples(), 0, 10),
		"count":  fmt.Sprintf(retrieveTemplatesCountStatementTemplate, coreFieldTriples()),
		"search": fmt.Sprintf(searchTemplatesStatementTemplate, coreFieldTriples(), "", 0, 10),
		"by id":  fmt.Sprintf(retrieveTemplateByIDStatementTemplate, coreFieldTriples(), "", ""),
	} {
		require.Contains(t, statement, `FILTER(?state IN ("registered", "published"))`, name)
	}
}
