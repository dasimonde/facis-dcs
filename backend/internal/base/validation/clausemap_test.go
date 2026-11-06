package validation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractClauseComponents(t *testing.T) {
	doc := map[string]any{
		"@id": "urn:contract:1",
		"dcs:documentStructure": map[string]any{
			"dcs:blocks": map[string]any{
				"@list": []any{
					map[string]any{
						"@id":       "urn:contract:1#block-a",
						"@type":     "dcs:Clause",
						"dcs:title": "Availability",
						"dcs:content": map[string]any{
							"@list": []any{
								"Availability is ",
								map[string]any{"@id": "urn:contract:1#field-availability"},
							},
						},
					},
					map[string]any{
						"@id":   "urn:contract:1#section",
						"@type": "dcs:Section",
					},
				},
			},
		},
	}
	clauses, err := ExtractClauseComponents(doc)
	require.NoError(t, err)
	require.Len(t, clauses, 1)
	require.Equal(t, "urn:contract:1#block-a", clauses[0].ID)
	require.Equal(t, "Availability", clauses[0].Title)
	require.Contains(t, clauses[0].Refs, "urn:contract:1#field-availability")
}

func TestMapFindingsToClausesConservative(t *testing.T) {
	clauses := []ClauseComponent{
		{ID: "urn:c#block-a", Title: "A", Refs: []string{"urn:c#field-a"}},
		{ID: "urn:c#block-b", Title: "B", Refs: []string{"urn:c#field-b"}},
	}

	t.Run("direct focus to clause id", func(t *testing.T) {
		mapped := MapFindingsToClauses([]PolicyFinding{{
			FocusNode: "urn:c#block-a",
			Message:   "x",
			Severity:  "error",
		}}, clauses, "urn:c")
		require.Equal(t, []string{"urn:c#block-a"}, mapped[0].ClauseIDs)
	})

	t.Run("document focus stays unassigned", func(t *testing.T) {
		mapped := MapFindingsToClauses([]PolicyFinding{{
			FocusNode: "urn:c",
			Message:   "x",
			Severity:  "error",
		}}, clauses, "urn:c")
		require.Empty(t, mapped[0].ClauseIDs)
	})

	t.Run("shared field ref stays unassigned", func(t *testing.T) {
		shared := []ClauseComponent{
			{ID: "urn:c#block-a", Refs: []string{"urn:c#shared"}},
			{ID: "urn:c#block-b", Refs: []string{"urn:c#shared"}},
		}
		mapped := MapFindingsToClauses([]PolicyFinding{{
			FocusNode: "urn:c#shared",
			Message:   "x",
			Severity:  "error",
		}}, shared, "urn:c")
		require.Empty(t, mapped[0].ClauseIDs)
	})

	t.Run("unique field focus maps to one clause", func(t *testing.T) {
		mapped := MapFindingsToClauses([]PolicyFinding{{
			FocusNode: "urn:c#field-a",
			Path:      "availability",
			Message:   "x",
			Severity:  "error",
		}}, clauses, "urn:c")
		require.Equal(t, []string{"urn:c#block-a"}, mapped[0].ClauseIDs)
	})
}
