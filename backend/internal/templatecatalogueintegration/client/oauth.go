package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const keycloakTokenPath = "/protocol/openid-connect/token"

// Config holds Federated Catalogue API and Keycloak client-credentials settings.
type Config struct {
	APIURL           string
	KeycloakRealmURL string
	ClientID         string
	ClientSecret     string
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
}

func clientCredentialsTokenURL(realmURL string) (string, error) {
	realm := strings.TrimSpace(realmURL)
	if realm == "" {
		return "", fmt.Errorf("keycloak realm url is empty")
	}
	parsed, err := url.Parse(strings.TrimRight(realm, "/"))
	if err != nil {
		return "", fmt.Errorf("invalid keycloak realm url: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("keycloak realm url must include scheme and host")
	}
	parsed.Path = strings.TrimSuffix(parsed.Path, "/") + keycloakTokenPath
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

// FetchAccessToken obtains a Federated Catalogue access token via Keycloak client_credentials.
func (c *FederatedCatalogueClient) FetchAccessToken(ctx context.Context) (string, error) {
	if c.tokenURL == "" {
		return "", fmt.Errorf("federated catalogue oauth is not configured")
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", c.clientID)
	form.Set("client_secret", c.clientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("create token request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read token response failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed tokenResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("unmarshal token response failed: %w", err)
	}
	token := strings.TrimSpace(parsed.AccessToken)
	if token == "" {
		return "", fmt.Errorf("token response missing access_token")
	}
	return token, nil
}
