package envelope_test

import (
	"math"
	"testing"

	"digital-contracting-service/internal/auth/oid4vp/status/envelope"

	"github.com/stretchr/testify/require"
)

func TestInt64Claim_MissingVsInvalid(t *testing.T) {
	t.Parallel()

	v, present, err := envelope.Int64Claim(map[string]any{}, "exp")
	require.NoError(t, err)
	require.False(t, present)
	require.Zero(t, v)

	_, present, err = envelope.Int64Claim(map[string]any{"exp": nil}, "exp")
	require.ErrorContains(t, err, "null")
	require.True(t, present)

	_, present, err = envelope.Int64Claim(map[string]any{"exp": "tomorrow"}, "exp")
	require.Error(t, err)
	require.True(t, present)
}

func TestInt64Claim_Boundaries(t *testing.T) {
	t.Parallel()

	max, present, err := envelope.Int64Claim(map[string]any{"iat": int64(math.MaxInt64)}, "iat")
	require.NoError(t, err)
	require.True(t, present)
	require.Equal(t, int64(math.MaxInt64), max)

	maxU, present, err := envelope.Int64Claim(map[string]any{"iat": uint64(math.MaxInt64)}, "iat")
	require.NoError(t, err)
	require.True(t, present)
	require.Equal(t, int64(math.MaxInt64), maxU)

	_, present, err = envelope.Int64Claim(map[string]any{"nbf": uint64(math.MaxInt64) + 1}, "nbf")
	require.ErrorContains(t, err, "int64 range")
	require.True(t, present)

	_, present, err = envelope.Int64Claim(map[string]any{"exp": uint64(math.MaxUint64)}, "exp")
	require.ErrorContains(t, err, "int64 range")
	require.True(t, present)

	_, present, err = envelope.Int64Claim(map[string]any{"exp": int64(-1)}, "exp")
	require.ErrorContains(t, err, "non-negative")
	require.True(t, present)
}

func TestInt64Claim_Float64EdgeCases(t *testing.T) {
	t.Parallel()

	ok, present, err := envelope.Int64Claim(map[string]any{"iat": float64(1719129600)}, "iat")
	require.NoError(t, err)
	require.True(t, present)
	require.Equal(t, int64(1719129600), ok)

	cases := []struct {
		name  string
		value any
		want  string
	}{
		{name: "nan", value: math.NaN(), want: "finite"},
		{name: "pos-inf", value: math.Inf(1), want: "finite"},
		{name: "neg-inf", value: math.Inf(-1), want: "finite"},
		{name: "fractional", value: 1.5, want: "whole number"},
		{name: "negative", value: float64(-1), want: "non-negative"},
		{name: "above-maxint64", value: float64(math.MaxInt64) * 2, want: "int64 range"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, present, err := envelope.Int64Claim(map[string]any{"exp": tc.value}, "exp")
			require.ErrorContains(t, err, tc.want)
			require.True(t, present)
		})
	}
}

func TestInt64Claim_Float64MaxInt64RoundsToTwoPow63(t *testing.T) {
	t.Parallel()

	// float64 cannot represent MaxInt64 exactly; conversion rounds to 2^63.
	rounded := float64(math.MaxInt64)
	require.Equal(t, float64(0x1p63), rounded)

	_, present, err := envelope.Int64Claim(map[string]any{"exp": rounded}, "exp")
	require.ErrorContains(t, err, "int64 range")
	require.True(t, present)

	_, present, err = envelope.Int64Claim(map[string]any{"exp": float64(0x1p63)}, "exp")
	require.ErrorContains(t, err, "int64 range")
	require.True(t, present)
}

func TestExpIatNbfFromClaims_DistinguishMissing(t *testing.T) {
	t.Parallel()

	claims := map[string]any{}
	_, present, err := envelope.ExpFromClaims(claims)
	require.NoError(t, err)
	require.False(t, present)
	_, present, err = envelope.NbfFromClaims(claims)
	require.NoError(t, err)
	require.False(t, present)
	_, present, err = envelope.IatFromClaims(claims)
	require.NoError(t, err)
	require.False(t, present)

	_, present, err = envelope.NbfFromClaims(map[string]any{"nbf": uint64(math.MaxUint64)})
	require.Error(t, err)
	require.True(t, present)
}
