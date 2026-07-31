// Package hydra implements OAuth2/OIDC and Hydra admin interactions for DCS login.
package hydra

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const defaultHTTPTimeout = 10 * time.Second

// Config holds Hydra issuer and DCS OAuth client settings.
type Config struct {
	PublicIssuerURL   string
	InternalIssuerURL string
	ClientID          string
	ClientSecret      string
	RedirectURI       string
	AdminURL          string
	HTTPTimeout       time.Duration
}

// Client talks to Hydra public OIDC endpoints and the admin API.
type Client struct {
	cfg        Config
	httpClient *http.Client
	metadataMu sync.RWMutex
	metadata   *ProviderMetadata
}

// ProviderMetadata is the subset of OpenID discovery used by DCS.
type ProviderMetadata struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	RevocationEndpoint    string `json:"revocation_endpoint"`
	EndSessionEndpoint    string `json:"end_session_endpoint"`
}

// TokenResponse is a Hydra token endpoint payload.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error,omitempty"`
	ErrorDesc    string `json:"error_description,omitempty"`
}

// New builds a Client from explicit configuration.
func New(cfg Config) *Client {
	cfg.PublicIssuerURL = strings.TrimRight(strings.TrimSpace(cfg.PublicIssuerURL), "/")
	cfg.InternalIssuerURL = strings.TrimRight(strings.TrimSpace(cfg.InternalIssuerURL), "/")
	if cfg.InternalIssuerURL == "" {
		cfg.InternalIssuerURL = cfg.PublicIssuerURL
	}
	cfg.ClientID = strings.TrimSpace(cfg.ClientID)
	cfg.ClientSecret = strings.TrimSpace(cfg.ClientSecret)
	cfg.RedirectURI = strings.TrimSpace(cfg.RedirectURI)
	if cfg.AdminURL == "" {
		cfg.AdminURL = strings.TrimRight(strings.TrimSpace(os.Getenv("HYDRA_ADMIN_URL")), "/")
	}
	if cfg.AdminURL == "" {
		cfg.AdminURL = "http://localhost:30085"
	}
	if cfg.HTTPTimeout <= 0 {
		cfg.HTTPTimeout = defaultHTTPTimeout
	}
	return &Client{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: cfg.HTTPTimeout},
	}
}

// IssuerURL returns the public Hydra issuer base URL.
func (c *Client) IssuerURL() string {
	return c.cfg.PublicIssuerURL
}

// PublicIssuerURL returns the edge-facing Hydra issuer base URL.
func (c *Client) PublicIssuerURL() string {
	return c.cfg.PublicIssuerURL
}

// InternalIssuerURL returns the in-cluster Hydra public API base URL.
func (c *Client) InternalIssuerURL() string {
	return c.cfg.InternalIssuerURL
}

func (c *Client) toInternalEndpoint(publicURL string) string {
	publicURL = strings.TrimSpace(publicURL)
	if publicURL == "" {
		return publicURL
	}
	if c.cfg.InternalIssuerURL == c.cfg.PublicIssuerURL {
		return publicURL
	}
	if strings.HasPrefix(publicURL, c.cfg.PublicIssuerURL) {
		return c.cfg.InternalIssuerURL + strings.TrimPrefix(publicURL, c.cfg.PublicIssuerURL)
	}
	return publicURL
}

// ClientID returns the OAuth client identifier.
func (c *Client) ClientID() string {
	return c.cfg.ClientID
}

// RedirectURI returns the configured OAuth2 redirect URI for this client.
func (c *Client) RedirectURI() string {
	return c.cfg.RedirectURI
}

// ProviderMetadata loads and caches OpenID discovery for the Hydra issuer.
func (c *Client) ProviderMetadata(ctx context.Context) (*ProviderMetadata, error) {
	if c.cfg.PublicIssuerURL == "" {
		return nil, fmt.Errorf("HYDRA_PUBLIC_ISSUER_URL is not configured")
	}

	c.metadataMu.RLock()

	if c.metadata != nil {
		cached := *c.metadata
		c.metadataMu.RUnlock()
		return &cached, nil
	}

	c.metadataMu.RUnlock()

	wellKnown := c.cfg.InternalIssuerURL + "/.well-known/openid-configuration"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wellKnown, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)

	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openid discovery status %d", resp.StatusCode)
	}

	var metadata ProviderMetadata
	if err := json.Unmarshal(body, &metadata); err != nil {
		return nil, err
	}

	if metadata.AuthorizationEndpoint == "" || metadata.TokenEndpoint == "" {
		return nil, fmt.Errorf("openid discovery missing authorization or token endpoint")
	}

	c.metadataMu.Lock()
	c.metadata = &metadata
	c.metadataMu.Unlock()

	return &metadata, nil
}

// EndSessionURL returns the OIDC end session endpoint from discovery.
func (c *Client) EndSessionURL(ctx context.Context) (string, error) {
	metadata, err := c.ProviderMetadata(ctx)

	if err != nil {
		return "", err
	}

	if metadata.EndSessionEndpoint == "" {
		return "", fmt.Errorf("openid discovery missing end_session_endpoint")
	}

	return metadata.EndSessionEndpoint, nil
}

// ExchangeCode exchanges an authorization code for tokens.
func (c *Client) ExchangeCode(ctx context.Context, code string) (*TokenResponse, error) {
	if strings.TrimSpace(c.cfg.ClientID) == "" || strings.TrimSpace(c.cfg.RedirectURI) == "" {
		return nil, fmt.Errorf("HYDRA_CLIENT_ID and HYDRA_REDIRECT_URI must be configured")
	}

	metadata, err := c.ProviderMetadata(ctx)
	if err != nil {
		return nil, err
	}

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("client_id", c.cfg.ClientID)

	if c.cfg.ClientSecret != "" {
		data.Set("client_secret", c.cfg.ClientSecret)
	}

	data.Set("redirect_uri", c.cfg.RedirectURI)

	return c.postTokenEndpoint(ctx, c.toInternalEndpoint(metadata.TokenEndpoint), data)
}

// RefreshAccessToken obtains a new access token from a refresh token.
func (c *Client) RefreshAccessToken(ctx context.Context, refreshToken string) (*TokenResponse, error) {
	if strings.TrimSpace(c.cfg.ClientID) == "" {
		return nil, fmt.Errorf("HYDRA_CLIENT_ID is not configured")
	}

	metadata, err := c.ProviderMetadata(ctx)

	if err != nil {
		return nil, err
	}

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)
	data.Set("client_id", c.cfg.ClientID)

	if c.cfg.ClientSecret != "" {
		data.Set("client_secret", c.cfg.ClientSecret)
	}

	return c.postTokenEndpoint(ctx, c.toInternalEndpoint(metadata.TokenEndpoint), data)
}

// RevokeRefreshToken revokes a refresh token at the provider revocation endpoint.
func (c *Client) RevokeRefreshToken(ctx context.Context, refreshToken string) error {
	if strings.TrimSpace(c.cfg.ClientID) == "" {
		return fmt.Errorf("HYDRA_CLIENT_ID is not configured")
	}

	metadata, err := c.ProviderMetadata(ctx)
	if err != nil || metadata.RevocationEndpoint == "" {
		return err
	}

	data := url.Values{}
	data.Set("token", refreshToken)
	data.Set("client_id", c.cfg.ClientID)

	if c.cfg.ClientSecret != "" {
		data.Set("client_secret", c.cfg.ClientSecret)
	}

	data.Set("token_type_hint", "refresh_token")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.toInternalEndpoint(metadata.RevocationEndpoint), strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("revoke failed with status %d", resp.StatusCode)
	}

	return nil
}

func (c *Client) postTokenEndpoint(ctx context.Context, tokenEndpoint string, data url.Values) (*TokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, err
	}

	if tokenResp.Error != "" {
		return nil, fmt.Errorf("%s: %s", tokenResp.Error, tokenResp.ErrorDesc)
	}

	return &tokenResp, nil
}
