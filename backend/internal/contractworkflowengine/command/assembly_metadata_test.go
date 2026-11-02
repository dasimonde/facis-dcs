package command

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/contractworkflowengine/db"
)

// DCS-FR-CWE-03: an assembled contract leaves draft only with its required
// metadata and a validity period that holds.
func TestAssemblyRequiresContractMetadata(t *testing.T) {
	text := func(s string) *string { return &s }
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	before := start.Add(-time.Hour)
	after := start.Add(time.Hour)
	negative := -1
	complete := func() db.Contract {
		return db.Contract{DID: "c1", Name: text("Service Agreement"), Description: text("Hosting"), StartDate: &start, ExpDate: &after}
	}
	for name, tc := range map[string]struct {
		edit    func(*db.Contract)
		problem string
	}{
		"complete":               {func(*db.Contract) {}, ""},
		"no dates":               {func(c *db.Contract) { c.StartDate, c.ExpDate = nil, nil }, ""},
		"no name":                {func(c *db.Contract) { c.Name = nil }, "name is missing"},
		"blank name":             {func(c *db.Contract) { c.Name = text("  ") }, "name is missing"},
		"no description":         {func(c *db.Contract) { c.Description = nil }, "description is missing"},
		"ends before start":      {func(c *db.Contract) { c.ExpDate = &before }, "expiration date is not after the start date"},
		"ends at start":          {func(c *db.Contract) { c.ExpDate = &start }, "expiration date is not after the start date"},
		"negative notice period": {func(c *db.Contract) { c.ExpNoticePeriod = &negative }, "notice period is negative"},
	} {
		t.Run(name, func(t *testing.T) {
			contract := complete()
			tc.edit(&contract)
			err := requireAssemblyMetadata(context.Background(), nil, &submitContractRepoFake{stored: &contract}, "c1")
			if tc.problem == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, ErrMissingRequiredMetadata)
			require.ErrorContains(t, err, tc.problem)
		})
	}
}
