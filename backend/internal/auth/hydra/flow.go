package hydra

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type acceptResp struct {
	RedirectTo string `json:"redirect_to"`
}

type loginAcceptReq struct {
	Subject     string         `json:"subject"`
	Remember    bool           `json:"remember"`
	RememberFor int            `json:"remember_for"`
	Context     map[string]any `json:"context,omitempty"`
}

type consentRequest struct {
	RequestedScope               []string       `json:"requested_scope"`
	RequestedAccessTokenAudience []string       `json:"requested_access_token_audience"`
	Context                      map[string]any `json:"context"`
}

type consentAcceptReq struct {
	GrantScope               []string `json:"grant_scope"`
	GrantAccessTokenAudience []string `json:"grant_access_token_audience"`
	Remember                 bool     `json:"remember"`
	RememberFor              int      `json:"remember_for"`
	Session                  struct {
		AccessToken map[string]any `json:"access_token,omitempty"`
		IDToken     map[string]any `json:"id_token,omitempty"`
	} `json:"session,omitempty"`
}

// AuthorizeURL builds the Hydra OAuth2 authorize URL (OIDC state = presentation state).
func (c *Client) AuthorizeURL(ctx context.Context, oidcState string) (string, error) {
	metadata, err := c.ProviderMetadata(ctx)
	if err != nil {
		return "", err
	}

	params := url.Values{}
	params.Set("client_id", c.cfg.ClientID)
	params.Set("redirect_uri", c.cfg.RedirectURI)
	params.Set("response_type", "code")
	params.Set("scope", "openid offline_access") // the combination of the open and offline scopes is the standard OpenID Connect method for requesting a Refresh Token
	params.Set("state", oidcState)
	params.Set("prompt", "login") // OID4VP login only needs a fresh login_challenge; avoid Hydra reusing a remembered browser session.
	return metadata.AuthorizationEndpoint + "?" + params.Encode(), nil
}

// AcceptConsent accepts a consent challenge via the Hydra admin API. The granted roles and
// issuer come exclusively from the consent request context, which Hydra copies from the
// login accept (see AcceptLoginAndConsent).
func (c *Client) AcceptConsent(ctx context.Context, challenge string) (string, error) {
	var consentReq consentRequest
	if err := c.getJSON(ctx, "/admin/oauth2/auth/requests/consent", url.Values{"consent_challenge": {challenge}}, &consentReq); err != nil {
		return "", err
	}

	rolesToGrant, err := rolesFromConsentContext(consentReq.Context)
	if err != nil {
		return "", err
	}
	iss, err := issuerFromConsentContext(consentReq.Context)
	if err != nil {
		return "", err
	}
	claims := tokenSessionClaims(rolesToGrant, iss)
	consentBody := consentAcceptReq{
		GrantScope:               consentReq.RequestedScope,
		GrantAccessTokenAudience: consentReq.RequestedAccessTokenAudience,
		Remember:                 true,
		RememberFor:              3600,
	}
	consentBody.Session.AccessToken = claims
	consentBody.Session.IDToken = claims

	var consentOut acceptResp
	if err := c.putJSON(ctx, "/admin/oauth2/auth/requests/consent/accept", url.Values{"consent_challenge": {challenge}}, consentBody, &consentOut); err != nil {
		return "", err
	}
	if consentOut.RedirectTo == "" {
		return "", fmt.Errorf("hydra consent accept returned empty redirect")
	}
	return c.ResolveRedirectChain(ctx, consentOut.RedirectTo)
}

// ResolveRedirectChain accepts nested Hydra consent redirects via the admin API until the
// redirect no longer carries login_challenge or consent_challenge query parameters.
func (c *Client) ResolveRedirectChain(ctx context.Context, redirectTo string) (string, error) {
	const maxSteps = 8
	for step := 0; step < maxSteps; step++ {
		redirectTo = strings.TrimSpace(redirectTo)
		if redirectTo == "" {
			return "", fmt.Errorf("hydra redirect chain: empty redirect")
		}
		u, err := url.Parse(redirectTo)
		if err != nil {
			return "", err
		}
		if cc := strings.TrimSpace(u.Query().Get("consent_challenge")); cc != "" {
			redirectTo, err = c.AcceptConsent(ctx, cc)
			if err != nil {
				return "", err
			}
			continue
		}
		if lc := strings.TrimSpace(u.Query().Get("login_challenge")); lc != "" {
			return "", fmt.Errorf("hydra redirect chain: unexpected login_challenge after login accept")
		}
		return redirectTo, nil
	}
	return "", fmt.Errorf("hydra redirect chain: exceeded %d steps", maxSteps)
}

func issuerFromConsentContext(ctx map[string]any) (string, error) {
	raw, _ := ctx["iss"].(string)
	iss := strings.TrimSpace(raw)
	if iss == "" {
		return "", fmt.Errorf("hydra consent context is missing iss")
	}
	return iss, nil
}

func rolesFromConsentContext(ctx map[string]any) ([]string, error) {
	arr, ok := ctx["roles"].([]any)
	if !ok || len(arr) == 0 {
		return nil, fmt.Errorf("hydra consent context is missing roles")
	}
	out := make([]string, 0, len(arr))
	for _, r := range arr {
		v, ok := r.(string)
		if !ok {
			return nil, fmt.Errorf("hydra consent context roles must be strings")
		}
		out = append(out, v)
	}
	return out, nil
}

// AcceptLoginAndConsent accepts login and consent via the Hydra admin API.
// organization and roles come from verified PoA claims.
func (c *Client) AcceptLoginAndConsent(ctx context.Context, challenge, subject, organization string, roles []string) (string, error) {
	loginCtx := map[string]any{
		"roles":  roles,
		"source": "openid4vp",
	}
	if iss := strings.TrimSpace(organization); iss != "" {
		loginCtx["iss"] = iss
	}
	loginReq := loginAcceptReq{
		Subject:     subject,
		Remember:    true,
		RememberFor: 3600,
		Context:     loginCtx,
	}

	var loginOut acceptResp
	if err := c.putJSON(ctx, "/admin/oauth2/auth/requests/login/accept", url.Values{"login_challenge": {challenge}}, loginReq, &loginOut); err != nil {
		return "", err
	}
	if loginOut.RedirectTo == "" {
		return "", fmt.Errorf("hydra login accept returned empty redirect")
	}

	// The redirect chain resolves any consent challenge from the login context.
	return c.ResolveRedirectChain(ctx, loginOut.RedirectTo)
}

func (c *Client) getJSON(ctx context.Context, path string, q url.Values, out any) error {
	return c.doJSON(ctx, http.MethodGet, path, q, nil, out)
}

func (c *Client) putJSON(ctx context.Context, path string, q url.Values, body any, out any) error {
	return c.doJSON(ctx, http.MethodPut, path, q, body, out)
}

func (c *Client) doJSON(ctx context.Context, method, path string, q url.Values, body any, out any) error {
	u := c.cfg.AdminURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}

	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}

	var bodyReader io.Reader
	if payload != nil {
		bodyReader = strings.NewReader(string(payload))
	}
	req, err := http.NewRequestWithContext(ctx, method, u, bodyReader)
	if err != nil {
		return err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errMsg, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("hydra %s %s failed: %d %s", method, path, resp.StatusCode, strings.TrimSpace(string(errMsg)))
	}

	if out == nil {
		return nil
	}

	msg, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("hydra %s %s read body: %w", method, path, err)
	}
	if len(msg) == 0 {
		return nil
	}
	return json.Unmarshal(msg, out)
}

// tokenSessionClaims maps verified PoA claims into Hydra session token claims.
func tokenSessionClaims(roles []string, iss string) map[string]any {
	claims := map[string]any{
		"roles": roles,
	}
	if iss := strings.TrimSpace(iss); iss != "" {
		claims["iss"] = iss
	}
	return claims
}
