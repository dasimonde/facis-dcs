package pg

import (
	"testing"

	"digital-contracting-service/internal/templaterepository/db"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateQueryInvalidatesCachedPdf(t *testing.T) {
	query, params, err := createQuery(db.ContractTemplateUpdateData{
		DID:  "did:example:template-1",
		Name: ptrString("Updated template"),
	})

	require.NoError(t, err)
	require.NotNil(t, query)
	assert.Contains(t, *query, "pdf_ipfs_cid = NULL")
	assert.Contains(t, *query, "pdf_renderer_version = NULL")
	if assert.Len(t, params, 2) {
		assert.IsType(t, (*string)(nil), params[0])
		assert.Equal(t, "Updated template", *params[0].(*string))
		assert.Equal(t, "did:example:template-1", params[1])
	}
}

func TestCreateQueryStateChangePreservesCachedPdf(t *testing.T) {
	query, params, err := createQuery(db.ContractTemplateUpdateData{
		DID:   "did:example:template-1",
		State: "SUBMITTED",
	})

	require.NoError(t, err)
	require.NotNil(t, query)
	assert.NotContains(t, *query, "pdf_ipfs_cid = NULL",
		"pure state transition must NOT clear the cached PDF CID so C2PA chain can be built")
	assert.Contains(t, *query, "state")
	if assert.Len(t, params, 2) {
		assert.Equal(t, "SUBMITTED", params[0])
		assert.Equal(t, "did:example:template-1", params[1])
	}
}

func ptrString(value string) *string {
	return &value
}
