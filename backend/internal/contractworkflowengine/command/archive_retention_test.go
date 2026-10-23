package command

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
)

// DCS-FR-CSA-14: retention runs from the contract's expiry.
func TestRetentionUntilStartsAtExpiry(t *testing.T) {
	stored := time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)
	exp := time.Date(2027, 10, 1, 0, 0, 0, 0, time.UTC)
	require.Equal(t, time.Date(2037, 10, 1, 0, 0, 0, 0, time.UTC), *retentionUntil(&exp, stored, 10))
}

func TestRetentionUntilStartsAtArchivalWithoutExpiry(t *testing.T) {
	stored := time.Date(2026, 10, 23, 0, 0, 0, 0, time.UTC)
	require.Equal(t, time.Date(2032, 10, 23, 0, 0, 0, 0, time.UTC), *retentionUntil(nil, stored, 6))
}

// DCS-FR-CSA-17: without a policy, deletion stays permitted.
func TestRetentionUntilWithoutPolicy(t *testing.T) {
	exp := time.Now()
	require.Nil(t, retentionUntil(&exp, time.Now(), 0))
}

func TestBuildArchiveEntryFillsTheRetentionDate(t *testing.T) {
	t.Setenv("DCS_ARCHIVE_RETENTION_YEARS", "10")
	now := time.Now().UTC()
	exp := time.Date(2027, 10, 1, 0, 0, 0, 0, time.UTC)
	entry, err := BuildArchiveEntry(&db.Contract{DID: "did:web:contract", ContractVersion: 1, State: contractstate.Signed.String(),
		CreatedAt: now, UpdatedAt: now, ExpDate: &exp}, "did:web:operator", ArchiveSigningEvidence{SignedAt: now})
	require.NoError(t, err)
	require.NotNil(t, entry.RetentionUntil)
	require.Equal(t, time.Date(2037, 10, 1, 0, 0, 0, 0, time.UTC), *entry.RetentionUntil)
}
