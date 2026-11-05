package semantichub

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNumericTypesAndTemporalValidation(t *testing.T) {
	for _, typ := range []string{"short", "byte", "nonNegativeInteger", "positiveInteger", "nonPositiveInteger", "negativeInteger", "unsignedLong", "unsignedInt", "unsignedShort", "unsignedByte"} {
		integer, _, _, ok := numericType(xsdNamespace + typ)
		require.True(t, integer && ok, typ)
	}
	require.True(t, validTypedValue(xsdNamespace+"nonNegativeInteger", float64(0)))
	require.False(t, validTypedValue(xsdNamespace+"nonNegativeInteger", float64(-1)))
	require.True(t, validTypedValue(xsdNamespace+"short", float64(-32768)))
	require.False(t, validTypedValue(xsdNamespace+"short", float64(-32769)))
	require.False(t, validTypedValue(xsdNamespace+"unsignedByte", float64(256)))
	require.False(t, validTypedValue(xsdNamespace+"integer", float64(1.5)))
	require.True(t, validTypedValue(xsdNamespace+"unsignedLong", json.Number("18446744073709551615")))
	require.True(t, validTypedValue(xsdNamespace+"unsignedLong", "18446744073709551615"))
	require.False(t, validTypedValue(xsdNamespace+"integer", "1/2"))
	require.False(t, validTypedValue(xsdNamespace+"unsignedLong", json.Number("18446744073709551616")))
	require.False(t, validTypedValue(xsdNamespace+"long", json.Number("9223372036854775808")))
	require.False(t, validTypedValue(xsdNamespace+"boolean", "false"))
	require.False(t, validTemporalValue(xsdNamespace+"date", "2026-02-30"))
	require.True(t, validTemporalValue(xsdNamespace+"date", "2026-11-04"))
	require.True(t, validTemporalValue(xsdNamespace+"date", "2026-11-04+01:00"))
	require.True(t, validTemporalValue(xsdNamespace+"dateTime", "2026-11-04T12:00:00+01:00"))
	require.True(t, validTemporalValue(xsdNamespace+"dateTime", "2026-11-04T12:00:00"))
	require.False(t, validTemporalValue(xsdNamespace+"dateTimeStamp", "2026-11-04T12:00:00"))
	require.False(t, validTemporalValue(xsdNamespace+"dateTime", "2026-11-04T27:00:00Z"))
	require.False(t, validTemporalValue(xsdNamespace+"dateTime", "2026-11-04T12:00:00+00:60"))
	require.False(t, validTemporalValue(xsdNamespace+"dateTime", "2026-11-04T12:00:00+14:01"))
}

func TestTextSearchTypes(t *testing.T) {
	for _, typ := range []string{"string", "normalizedString", "token", "language", "Name", "NCName", "NMTOKEN"} {
		require.True(t, textSearchType(xsdNamespace+typ, false), typ)
		require.False(t, textSearchType(xsdNamespace+typ, true), typ)
	}
	for _, typ := range []string{"anyURI", "hexBinary", "base64Binary", "integer"} {
		require.False(t, textSearchType(xsdNamespace+typ, false), typ)
	}
}
