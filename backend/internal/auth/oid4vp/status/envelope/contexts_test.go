package envelope

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

type failingRoundTripper struct{}

func (failingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("network down")
}

func TestFetchingContextLoader_FallsBackToEmbeddedOnNetworkError(t *testing.T) {
	loader := &fetchingContextLoader{
		httpClient: &http.Client{Transport: failingRoundTripper{}},
		remote:     make(map[string]map[string]any),
	}

	// On timeout or network error, fall back to contexts/data-integrity-v2.json.
	doc, err := loader.LoadDocument(dataIntegrityV2ContextURL)
	require.NoError(t, err)
	payload, ok := doc.Document.(map[string]any)
	require.True(t, ok)
	require.NotNil(t, payload["@context"])
}
