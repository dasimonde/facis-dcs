package pg

import (
	"strings"
	"testing"
	"time"

	"digital-contracting-service/internal/contractworkflowengine/db"

	"github.com/stretchr/testify/require"
)

func TestCreateRemoteQueryOmitsZeroCreatedAt(t *testing.T) {
	query, params, err := createRemoteQuery(db.Contract{
		DID:       "did:web:example:contract",
		State:     "NEGOTIATION",
		UpdatedAt: time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC),
		CreatedBy: "did:web:example",
		Origin:    "did:web:example",
	}, nil)
	require.NoError(t, err)
	require.NotNil(t, query)
	require.NotContains(t, *query, "created_at")
	require.Contains(t, *query, "updated_at")
	require.Equal(t, "did:web:example:contract", params[len(params)-1])
}

func TestCreateRemoteQueryIncludesNonZeroCreatedAt(t *testing.T) {
	created := time.Date(2026, 10, 21, 11, 0, 0, 0, time.UTC)
	query, params, err := createRemoteQuery(db.Contract{
		DID:       "did:web:example:contract",
		State:     "NEGOTIATION",
		CreatedAt: created,
		UpdatedAt: time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC),
		CreatedBy: "did:web:example",
		Origin:    "did:web:example",
	}, nil)
	require.NoError(t, err)
	require.NotNil(t, query)
	require.True(t, strings.Contains(*query, "created_at"))
	require.Contains(t, params, created)
}
