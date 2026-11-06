package main

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRequestDecoderPreservesJSONNumbers(t *testing.T) {
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"data":{"value":18446744073709551615}}`))
	r.Header.Set("Content-Type", "application/json")
	var payload struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, requestDecoderWithForm(r).Decode(&payload))
	require.Equal(t, json.Number("18446744073709551615"), payload.Data["value"])
}
