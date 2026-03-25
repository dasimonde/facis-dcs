package client

import (
	"bytes"
	"context"

	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Response wraps essential HTTP response data.
type Response struct {
	StatusCode int
	Body       []byte
	Headers    http.Header
}

// FederatedCatalogueClient handles outbound requests to Federated Catalogue.
type FederatedCatalogueClient struct {
	baseURL    string
	httpClient *http.Client
}

const ParticipantsEndpointPath = "/participants"

// NewFederatedCatalogueClient creates a Federated Catalogue client.
func NewFederatedCatalogueClient(apiURL string) *FederatedCatalogueClient {
	return &FederatedCatalogueClient{
		baseURL: normalizeBaseURL(apiURL),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// BaseURL returns the normalized configured API URL.
func (c *FederatedCatalogueClient) BaseURL() string {
	return c.baseURL
}

// Post sends a POST request to Federated Catalogue.
func (c *FederatedCatalogueClient) Post(ctx context.Context, path string, bearerToken string, query url.Values, body []byte) (*Response, error) {
	return c.doRequest(ctx, http.MethodPost, path, bearerToken, query, body)
}

func (c *FederatedCatalogueClient) doRequest(ctx context.Context, method string, path string, bearerToken string, query url.Values, body []byte) (*Response, error) {
	if c.baseURL == "" {
		return nil, fmt.Errorf("federated catalogue api url is empty")
	}

	requestURL, err := url.Parse(c.baseURL + path)
	if err != nil {
		return nil, fmt.Errorf("invalid request url: %w", err)
	}
	if query != nil {
		requestURL.RawQuery = query.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, method, requestURL.String(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	token := strings.TrimSpace(bearerToken)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read the response body and limit the size to 1MB.
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read response failed: %w", err)
	}

	return &Response{
		StatusCode: resp.StatusCode,
		Body:       respBody,
		Headers:    resp.Header.Clone(),
	}, nil
}

func normalizeBaseURL(v string) string {
	trimmed := strings.TrimSpace(v)
	return strings.TrimRight(trimmed, "/")
}

// TODO: replace with the actual verification method
// BuildProof returns a proof template.
func (c *FederatedCatalogueClient) BuildProof(document map[string]interface{}, proofPurpose string) map[string]interface{} {
	now := time.Now().UTC()

	proof := map[string]interface{}{
		"type":               "JsonWebSignature2020",
		"created":            now.Format(time.RFC3339),
		"proofPurpose":       proofPurpose,
		"verificationMethod": "did:web:argo.asd-stack.eu#key-1",
		"jws":                "eyJhbGciOiJSUzI1NiIsImI2NCI6ZmFsc2UsImNyaXQiOlsiYjY0Il19..kTCYt5XsITJX1CxPCT8yAV-TVIw5WEuts01mqpQy7UJiN5mgREEMGlv50aqzpqh4Qq_PbChOMqsLfRoPsnsgxD-WUcX16dUOqV0G_zS245-kronKb78cPktb3rk-BuQy72IFLN25DYuNzVBAh4vGHSrQyHUGlcTwLtjPAnKb78",
	}
	if proofPurpose == "authentication" {
		proof["challenge"] = "1f44d55f-f161-4938-a659-f8026467f126"
		proof["domain"] = "4jt78h47fh47"
	}
	return proof
}
