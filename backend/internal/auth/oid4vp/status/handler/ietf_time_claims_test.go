package handler

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestValidateTimeClaims_FailClosedOnMalformedClaims(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_719_129_600, 0).UTC()
	h := &IETFToken{Now: func() time.Time { return now }}

	t.Run("missing optional claims stay optional", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, h.validateTimeClaims(map[string]any{
			"iat": now.Unix(),
		}))
	})

	t.Run("present invalid nbf is rejected", func(t *testing.T) {
		t.Parallel()
		err := h.validateTimeClaims(map[string]any{
			"nbf": uint64(math.MaxUint64),
		})
		require.ErrorContains(t, err, "nbf")
	})

	t.Run("present invalid exp is rejected", func(t *testing.T) {
		t.Parallel()
		err := h.validateTimeClaims(map[string]any{
			"exp": math.NaN(),
		})
		require.ErrorContains(t, err, "exp")
	})

	t.Run("present invalid iat is rejected", func(t *testing.T) {
		t.Parallel()
		err := h.validateTimeClaims(map[string]any{
			"iat": "yesterday",
		})
		require.ErrorContains(t, err, "iat")
	})

	t.Run("overflowed nbf does not fail open", func(t *testing.T) {
		t.Parallel()
		// Pre-fix behaviour wrapped MaxUint64 to a negative int64, so
		// now < nbf was false and the token was accepted. Fail closed instead.
		err := h.validateTimeClaims(map[string]any{
			"nbf": uint64(math.MaxInt64) + 1,
		})
		require.Error(t, err)
	})

	t.Run("valid window still accepted", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, h.validateTimeClaims(map[string]any{
			"iat": now.Add(-time.Minute).Unix(),
			"nbf": now.Add(-time.Second).Unix(),
			"exp": now.Add(time.Hour).Unix(),
		}))
	})
}
