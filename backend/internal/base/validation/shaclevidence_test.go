package validation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSHACLEvidenceIsStableAndDetectsDrift(t *testing.T) {
	contract := canonicalAuditContract()

	version, hash, err := SHACLEvidence(context.Background(), contract)
	require.NoError(t, err)
	require.Equal(t, 1, version)
	require.NotEmpty(t, hash)

	// Same document, revalidated again: identical hash (Phase 4 drift check
	// baseline — re-running validation on an unmutated document must not
	// spuriously report drift).
	_, hashAgain, err := SHACLEvidence(context.Background(), contract)
	require.NoError(t, err)
	require.Equal(t, hash, hashAgain)

	// Mutate the document after evidence was produced: the hash changes,
	// which is exactly the drift signal Phase 4's re-verification compares
	// against the embedded one.
	contract["dcs:metadata"] = map[string]any{"@type": "dcs:ContractMetadata", "dcs:version": 1}
	_, mutatedHash, err := SHACLEvidence(context.Background(), contract)
	require.NoError(t, err)
	require.NotEqual(t, hash, mutatedHash)
}
