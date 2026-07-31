package hydra

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// OAuth2Client is the subset of Hydra's admin client representation DCS uses.
// Secret is populated only by Hydra's create and rotate responses: Hydra keeps
// a hash and cannot return the value again, which is what makes the
// show-it-once contract enforceable rather than a convention.
type OAuth2Client struct {
	ClientID   string   `json:"client_id"`
	Secret     string   `json:"client_secret,omitempty"`
	Name       string   `json:"client_name,omitempty"`
	GrantTypes []string `json:"grant_types,omitempty"`
	Scope      string   `json:"scope,omitempty"`
	AuthMethod string   `json:"token_endpoint_auth_method,omitempty"`
}

// ErrOAuth2ClientNotFound is returned when Hydra has no client under that id.
var ErrOAuth2ClientNotFound = fmt.Errorf("oauth2 client not found")

// generateSecret returns a 256-bit secret. Callers hand it to the operator once
// and never store it: Hydra holds only a hash.
func generateSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("could not generate a client secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// CreateMachineClient registers a client_credentials OAuth2 client and returns
// it with the generated secret. The secret is readable only from this response.
func (c *Client) CreateMachineClient(ctx context.Context, clientID, name string) (*OAuth2Client, error) {
	secret, err := generateSecret()
	if err != nil {
		return nil, err
	}

	// No response_types and no scope: a machine caller never runs a browser
	// flow, and what it may do is decided by the registry rather than asked for
	// in the token.
	body := OAuth2Client{
		ClientID:   clientID,
		Secret:     secret,
		Name:       name,
		GrantTypes: []string{"client_credentials"},
		AuthMethod: "client_secret_post",
	}

	var created OAuth2Client
	if err := c.adminJSON(ctx, http.MethodPost, "/admin/clients", body, &created); err != nil {
		return nil, err
	}
	// Hydra echoes the secret on create, but not on any later read.
	if strings.TrimSpace(created.Secret) == "" {
		created.Secret = secret
	}
	return &created, nil
}

// RotateMachineClientSecret replaces the client's secret and returns the new
// one. The previous secret stops working as soon as this returns, so an
// operator must reconfigure the integration with what it hands back.
func (c *Client) RotateMachineClientSecret(ctx context.Context, clientID string) (string, error) {
	secret, err := generateSecret()
	if err != nil {
		return "", err
	}

	existing, err := c.GetMachineClient(ctx, clientID)
	if err != nil {
		return "", err
	}

	// PUT replaces the registration, so the grant types and auth method have to
	// be restated or Hydra would reset them to its own defaults.
	body := OAuth2Client{
		ClientID:   clientID,
		Secret:     secret,
		Name:       existing.Name,
		GrantTypes: existing.GrantTypes,
		AuthMethod: existing.AuthMethod,
	}
	if len(body.GrantTypes) == 0 {
		body.GrantTypes = []string{"client_credentials"}
	}
	if body.AuthMethod == "" {
		body.AuthMethod = "client_secret_post"
	}

	if err := c.adminJSON(ctx, http.MethodPut, "/admin/clients/"+clientID, body, nil); err != nil {
		return "", err
	}
	return secret, nil
}

// GetMachineClient reads a client registration. The secret is never part of it.
func (c *Client) GetMachineClient(ctx context.Context, clientID string) (*OAuth2Client, error) {
	var found OAuth2Client
	if err := c.adminJSON(ctx, http.MethodGet, "/admin/clients/"+clientID, nil, &found); err != nil {
		return nil, err
	}
	return &found, nil
}

// DeleteMachineClient removes the registration, so a credential cannot outlive
// the registry entry that justified it. A client Hydra does not have is treated
// as already gone.
func (c *Client) DeleteMachineClient(ctx context.Context, clientID string) error {
	err := c.adminJSON(ctx, http.MethodDelete, "/admin/clients/"+clientID, nil, nil)
	if err != nil && !isNotFound(err) {
		return err
	}
	return nil
}

func isNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "status 404")
}

func (c *Client) adminJSON(ctx context.Context, method, path string, in any, out any) error {
	adminURL := strings.TrimRight(strings.TrimSpace(c.cfg.AdminURL), "/")
	if adminURL == "" {
		return fmt.Errorf("HYDRA_ADMIN_URL is not configured")
	}

	var reader io.Reader
	if in != nil {
		encoded, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("could not encode the %s request: %w", path, err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, adminURL+path, reader)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("hydra admin %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("could not read the hydra admin response: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("hydra admin %s %s: status 404: %w", method, path, ErrOAuth2ClientNotFound)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("hydra admin %s %s: status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(payload)))
	}

	if out != nil && len(payload) > 0 {
		if err := json.Unmarshal(payload, out); err != nil {
			return fmt.Errorf("could not decode the hydra admin response: %w", err)
		}
	}
	return nil
}
