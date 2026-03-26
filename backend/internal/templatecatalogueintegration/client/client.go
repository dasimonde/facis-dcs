package client

import (
	"bytes"
	"context"
	"encoding/json"

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

// QueryResults matches the JSON body returned by FC /query.
type QueryResults struct {
	TotalCount int                      `json:"totalCount"`
	Items      []map[string]interface{} `json:"items"`
}

// QueryRequest is the JSON payload sent to FC /query.
type QueryRequest struct {
	Statement  string            `json:"statement"`
	Parameters map[string]string `json:"parameters"`
}

type SelfDescriptionMeta struct {
	SdHash string `json:"sdHash"`
	ID     string `json:"id"`
}

type SelfDescriptionResult struct {
	Meta    SelfDescriptionMeta `json:"meta"`
	Content *string             `json:"content"`
}

type GetSelfDescriptionsResponse struct {
	TotalCount int                     `json:"totalCount"`
	Items      []SelfDescriptionResult `json:"items"`
}

type GetSelfDescriptionsRequest struct {
	IDs         []string
	WithContent bool
}

// FederatedCatalogueClient handles outbound requests to Federated Catalogue.
type FederatedCatalogueClient struct {
	baseURL    string
	httpClient *http.Client
}

const ParticipantsEndpointPath = "/participants"
const SelfDescriptionsEndpointPath = "/self-descriptions"
const QueryEndpointPath = "/query"

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

// Query sends an FC /query request and decodes the JSON response.
func (c *FederatedCatalogueClient) Query(ctx context.Context, bearerToken string, req QueryRequest) (*QueryResults, error) {
	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal /query request failed: %w", err)
	}

	resp, err := c.Post(ctx, QueryEndpointPath, bearerToken, nil, bodyBytes)
	if err != nil {
		return nil, err
	}

	var results QueryResults
	if err := json.Unmarshal(resp.Body, &results); err != nil {
		return nil, fmt.Errorf("unmarshal /query response failed: %w", err)
	}
	return &results, nil
}

func (c *FederatedCatalogueClient) GetSelfDescriptions(ctx context.Context, bearerToken string, req GetSelfDescriptionsRequest) (*GetSelfDescriptionsResponse, error) {
	query := url.Values{}
	if len(req.IDs) > 0 {
		query.Set("ids", strings.Join(req.IDs, ","))
	}
	// withContent default is false in FC API
	if req.WithContent {
		query.Set("withContent", "true")
	}
	query.Set("withMeta", "true")

	resp, err := c.Get(ctx, SelfDescriptionsEndpointPath, bearerToken, query)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("get self-descriptions failed with status %d", resp.StatusCode)
	}

	var out GetSelfDescriptionsResponse
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		return nil, fmt.Errorf("unmarshal self-descriptions response failed: %w", err)
	}
	return &out, nil
}

// Put sends a PUT request to Federated Catalogue.
func (c *FederatedCatalogueClient) Put(ctx context.Context, path string, bearerToken string, query url.Values, body []byte) (*Response, error) {
	return c.doRequest(ctx, http.MethodPut, path, bearerToken, query, body)
}

// Get sends a GET request to Federated Catalogue.
func (c *FederatedCatalogueClient) Get(ctx context.Context, path string, bearerToken string, query url.Values) (*Response, error) {
	return c.doRequest(ctx, http.MethodGet, path, bearerToken, query, nil)
}

// Delete sends a DELETE request to Federated Catalogue.
func (c *FederatedCatalogueClient) Delete(ctx context.Context, path string, bearerToken string, query url.Values) (*Response, error) {
	return c.doRequest(ctx, http.MethodDelete, path, bearerToken, query, nil)
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
