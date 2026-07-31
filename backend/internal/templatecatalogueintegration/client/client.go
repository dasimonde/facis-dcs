package client

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Readiness fixture from the Apache-2.0 upstream Federated Catalogue
// fc-service-server test suite at revision c243a65955a16ff8bbc3a24deacfa8fa9a2192ca:
// src/test/resources/mock-data/default-participant.json. Keeping the upstream
// known-good credential as an embedded source avoids inventing a second DCS
// domain payload solely for lifecycle probing.
//
//go:embed fixtures/fc-readiness-participant.json
var readinessVerificationPayload []byte

var (
	ErrFederatedCatalogueNotConfigured      = errors.New("federated catalogue is not configured")
	ErrTemplateNotFoundInFederatedCatalogue = errors.New("template not found in Federated Catalogue")
)

// Response wraps essential HTTP response data.
type Response struct {
	StatusCode int
	Body       []byte
	Headers    http.Header
}

// AssetMeta is FC asset metadata returned by GET /assets.
type AssetMeta struct {
	AssetHash   string `json:"assetHash"`
	ID          string `json:"id"`
	ContentKind string `json:"contentKind"`
	ContentType string `json:"contentType"`
	Issuer      string `json:"issuer"`
	Status      string `json:"status"`
}

// AssetResult is a single asset entry from GET /assets.
type AssetResult struct {
	Meta    AssetMeta `json:"meta"`
	Content *string   `json:"content"`
}

// GetAssetsResponse is the paginated GET /assets response body.
type GetAssetsResponse struct {
	TotalCount int           `json:"totalCount"`
	Items      []AssetResult `json:"items"`
}

// GetAssetsRequest configures GET /assets query parameters.
type GetAssetsRequest struct {
	IDs         []string
	WithContent bool
	Offset      int
	Limit       int
}

// QueryResults is the JSON body returned by FC POST /query/search.
type QueryResults struct {
	TotalCount int                      `json:"totalCount"`
	Items      []map[string]interface{} `json:"items"`
}

// QueryRequest carries a SPARQL query sent to FC POST /query/search.
type QueryRequest struct {
	Statement string
}

type fcErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

const (
	JSONContentType   = "application/json"
	JSONLDContentType = "application/ld+json"
	RDFXMLContentType = "application/rdf+xml"
)

// FederatedCatalogueClient handles outbound requests to Federated Catalogue.
type FederatedCatalogueClient struct {
	baseURL      string
	tokenURL     string
	clientID     string
	clientSecret string
	httpClient   *http.Client
}

const (
	AssetsEndpointPath       = "/assets"
	HealthEndpointPath       = "/actuator/health"
	SchemaEndpointPath       = "/schemas"
	VerificationEndpointPath = "/verification"
	QuerySearchEndpointPath  = "/query/search"
)

// CheckReadiness performs exactly one functional verification request before
// DCS may use the catalogue. For a co-deployed FC, requireNativeHealth adds the
// native actuator check first. Remote catalogues skip that deployment-specific
// check because operators commonly do not expose actuator endpoints.
func (c *FederatedCatalogueClient) CheckReadiness(ctx context.Context, requireNativeHealth bool) error {
	if requireNativeHealth {
		healthURL, err := url.Parse(c.baseURL + HealthEndpointPath)
		if err != nil {
			return fmt.Errorf("invalid federated catalogue health url: %w", err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL.String(), nil)
		if err != nil {
			return fmt.Errorf("create federated catalogue health request: %w", err)
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("federated catalogue health check failed: %w", err)
		}
		healthBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		closeErr := resp.Body.Close()
		if readErr != nil {
			return fmt.Errorf("read federated catalogue health response: %w", readErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close federated catalogue health response: %w", closeErr)
		}
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			return fmt.Errorf("federated catalogue health check failed with status %d: %s",
				resp.StatusCode, strings.TrimSpace(string(healthBody)))
		}
		var health struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(healthBody, &health); err != nil {
			return fmt.Errorf("decode federated catalogue health response: %w", err)
		}
		if !strings.EqualFold(strings.TrimSpace(health.Status), "UP") {
			return fmt.Errorf("federated catalogue health is %q, expected UP", health.Status)
		}
	}

	query := url.Values{}
	query.Set("verifySchema", "true")
	query.Set("verifySemantics", "true")
	query.Set("verifySignatures", "false")
	query.Set("verifyVCSignature", "false")
	query.Set("verifyVPSignature", "false")
	verification, err := c.Post(ctx, VerificationEndpointPath, query, readinessVerificationPayload)
	if err != nil {
		return fmt.Errorf("federated catalogue functional verification failed: %w", err)
	}
	if verification.StatusCode < http.StatusOK || verification.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("federated catalogue functional verification failed with status %d: %s",
			verification.StatusCode, strings.TrimSpace(string(verification.Body)))
	}
	var result struct {
		VerificationTimestamp string         `json:"verificationTimestamp"`
		LifecycleStatus       string         `json:"lifecycleStatus"`
		Issuer                string         `json:"issuer"`
		IssuedDateTime        string         `json:"issuedDateTime"`
		BaseClass             string         `json:"baseClass"`
		Claims                map[string]any `json:"claims"`
	}
	if err := json.Unmarshal(verification.Body, &result); err != nil {
		return fmt.Errorf("decode federated catalogue functional verification result: %w", err)
	}
	if result.Issuer != "did:example:issuer" ||
		result.IssuedDateTime != "2010-01-01T19:23:24Z" ||
		strings.TrimSpace(result.VerificationTimestamp) == "" ||
		strings.TrimSpace(result.LifecycleStatus) == "" ||
		strings.TrimSpace(result.BaseClass) == "" ||
		len(result.Claims) == 0 {
		return fmt.Errorf("federated catalogue functional verification returned incomplete semantic result")
	}
	return nil
}

func NewFederatedCatalogueClient(cfg Config) (*FederatedCatalogueClient, error) {
	apiURL := normalizeBaseURL(cfg.APIURL)
	if apiURL == "" {
		return nil, nil
	}

	realmURL := strings.TrimSpace(cfg.KeycloakRealmURL)
	clientID := strings.TrimSpace(cfg.ClientID)
	clientSecret := strings.TrimSpace(cfg.ClientSecret)
	if realmURL == "" || clientID == "" || clientSecret == "" {
		return nil, fmt.Errorf("federated catalogue client requires KeycloakRealmURL, ClientID, and ClientSecret when APIURL is set")
	}

	tokenURL, err := clientCredentialsTokenURL(realmURL)
	if err != nil {
		return nil, err
	}

	return &FederatedCatalogueClient{
		baseURL:      apiURL,
		tokenURL:     tokenURL,
		clientID:     clientID,
		clientSecret: clientSecret,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// BaseURL returns the normalized configured API URL.
func (c *FederatedCatalogueClient) BaseURL() string {
	return c.baseURL
}

// Post sends a POST request to Federated Catalogue.
func (c *FederatedCatalogueClient) Post(ctx context.Context, path string, query url.Values, body []byte) (*Response, error) {
	return c.doRequest(ctx, http.MethodPost, path, query, JSONContentType, JSONContentType, body)
}

// PostRaw sends a POST with an explicit Content-Type.
func (c *FederatedCatalogueClient) PostRaw(ctx context.Context, path string, query url.Values, contentType string, body []byte) (*Response, error) {
	return c.doRequest(ctx, http.MethodPost, path, query, contentType, JSONContentType, body)
}

// GetAssets fetches asset metadata (and optionally content) from GET /assets.
func (c *FederatedCatalogueClient) GetAssets(ctx context.Context, req GetAssetsRequest) (*GetAssetsResponse, error) {
	query := url.Values{}
	if len(req.IDs) > 0 {
		query.Set("ids", strings.Join(req.IDs, ","))
	}
	if req.WithContent {
		query.Set("withContent", "true")
	}
	if req.Offset > 0 {
		query.Set("offset", fmt.Sprintf("%d", req.Offset))
	}
	if req.Limit > 0 {
		query.Set("limit", fmt.Sprintf("%d", req.Limit))
	}
	query.Set("withMeta", "true")

	resp, err := c.Get(ctx, AssetsEndpointPath, query)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("get assets failed with status %d", resp.StatusCode)
	}

	var out GetAssetsResponse
	if err := json.Unmarshal(resp.Body, &out); err != nil {
		return nil, fmt.Errorf("unmarshal assets response failed: %w", err)
	}
	return &out, nil
}

// Query executes a SPARQL graph query via FC POST /query/search. FC's
// deployed graph store is Fuseki (SPARQL-only) — see
// deployment/helm/charts/federated-catalogue.
func (c *FederatedCatalogueClient) Query(ctx context.Context, req QueryRequest) (*QueryResults, error) {
	statement := strings.TrimSpace(req.Statement)
	if statement == "" {
		return nil, fmt.Errorf("query statement is empty")
	}

	body := map[string]any{
		"statement": statement,
		"annotations": map[string]any{
			"queryLanguage":  "SPARQL",
			"withTotalCount": true,
		},
	}

	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal /query/search request failed: %w", err)
	}

	resp, err := c.Post(ctx, QuerySearchEndpointPath, nil, raw)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, c.queryHTTPError(resp)
	}

	var results QueryResults
	if err := json.Unmarshal(resp.Body, &results); err != nil {
		return nil, fmt.Errorf("unmarshal /query/search response failed: %w", err)
	}

	return &results, nil
}

// Put sends a PUT request to Federated Catalogue.
func (c *FederatedCatalogueClient) Put(ctx context.Context, path string, query url.Values, body []byte) (*Response, error) {
	return c.doRequest(ctx, http.MethodPut, path, query, JSONContentType, JSONContentType, body)
}

// PutRaw sends a PUT with an explicit Content-Type.
func (c *FederatedCatalogueClient) PutRaw(ctx context.Context, path string, query url.Values, contentType string, body []byte) (*Response, error) {
	return c.doRequest(ctx, http.MethodPut, path, query, contentType, JSONContentType, body)
}

// Get sends a GET request to Federated Catalogue.
func (c *FederatedCatalogueClient) Get(ctx context.Context, path string, query url.Values) (*Response, error) {
	return c.doRequest(ctx, http.MethodGet, path, query, JSONContentType, JSONContentType, nil)
}

// Delete sends a DELETE request to Federated Catalogue.
func (c *FederatedCatalogueClient) Delete(ctx context.Context, path string, query url.Values) (*Response, error) {
	return c.doRequest(ctx, http.MethodDelete, path, query, JSONContentType, JSONContentType, nil)
}

func (c *FederatedCatalogueClient) doRequest(ctx context.Context, method string, path string, query url.Values, contentType string, accept string, body []byte) (*Response, error) {
	if c.baseURL == "" {
		return nil, fmt.Errorf("federated catalogue api url is empty")
	}

	token, err := c.FetchAccessToken(ctx)
	if err != nil {
		return nil, err
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
	if contentType == "" {
		contentType = JSONContentType
	}
	if accept == "" {
		accept = JSONContentType
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", accept)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Println("could not close response body")
		}
	}(resp.Body)

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

// ExtractErrorMessage tries to extract an error message from the response body of a failed FC request.
func (c *FederatedCatalogueClient) ExtractErrorMessage(body []byte) string {
	_ = c
	var errBody fcErrorBody
	if err := json.Unmarshal(body, &errBody); err != nil {
		return ""
	}
	return errBody.Message
}

// ExtractErrorCode tries to extract the FC error code from a response body.
func (c *FederatedCatalogueClient) ExtractErrorCode(body []byte) string {
	_ = c
	var errBody fcErrorBody
	if err := json.Unmarshal(body, &errBody); err != nil {
		return ""
	}
	return errBody.Code
}

func (c *FederatedCatalogueClient) queryHTTPError(resp *Response) error {
	code := c.ExtractErrorCode(resp.Body)
	msg := c.ExtractErrorMessage(resp.Body)

	if code != "" && msg != "" {
		return fmt.Errorf("/query/search failed with status %d: %s: %s", resp.StatusCode, code, msg)
	}

	if msg != "" {
		return fmt.Errorf("/query/search failed with status %d: %s", resp.StatusCode, msg)
	}

	return fmt.Errorf("/query/search failed with status %d", resp.StatusCode)
}

// SchemaHTTPError formats a failed FC /schemas HTTP response as an error.
func (c *FederatedCatalogueClient) SchemaHTTPError(action string, resp *Response) error {
	code := c.ExtractErrorCode(resp.Body)
	msg := c.ExtractErrorMessage(resp.Body)
	if code != "" && msg != "" {
		return fmt.Errorf("%s failed with status %d: %s: %s", action, resp.StatusCode, code, msg)
	}
	if msg != "" {
		return fmt.Errorf("%s failed with status %d: %s", action, resp.StatusCode, msg)
	}
	return fmt.Errorf("%s failed with status %d", action, resp.StatusCode)
}
