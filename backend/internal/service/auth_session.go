package service

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"time"

	genauth "digital-contracting-service/gen/auth"
	authaudit "digital-contracting-service/internal/auth/audit"
	authdb "digital-contracting-service/internal/auth/db"
	"digital-contracting-service/internal/auth/hydra"
	"digital-contracting-service/internal/auth/oid4vp"
	oid4vprequest "digital-contracting-service/internal/auth/oid4vp/request"

	"github.com/jmoiron/sqlx"
	"goa.design/clue/log"
	goa "goa.design/goa/v3/pkg"
)

const oauthStateSizeBytes = 24

// AuthConfig wires auth dependencies from cmd/dcs/main.go (no env reads in handlers).
type AuthConfig struct {
	Hydra         *hydra.Client
	Trust         *oid4vp.TrustConfig
	DCQLQuery     any
	PIDDCQLQuery  any
	RequestSigner oid4vprequest.Signer
	// OID4VPClientID identifies this verifier to a wallet. It is NOT the Hydra
	// OAuth client id: OpenID4VP requires a prefixed identifier, and a bare
	// value means the "pre-registered" prefix, which wallets outside a
	// pre-agreed federation refuse outright.
	OID4VPClientID string
	// ParticipantDID is this instance's own participant identity. A login
	// credential attests authority for one organization, and an instance only
	// grants access to itself — see the login check in auth_login.go.
	ParticipantDID    string
	PublicAPIBase     string
	LogoutRedirectURI string
	UIPath            string
	OID4VPStateTTL    time.Duration
}

type authSvc struct {
	hydra             *hydra.Client
	trust             *oid4vp.TrustConfig
	dcqlQuery         any
	pidDCQLQuery      any
	logoutRedirectURI string
	uiBasePath        string
	publicAPIBase     string
	oid4vpStateTTL    time.Duration
	presentations     authdb.PresentationAttemptRepo
	requestSigner     oid4vprequest.Signer
	oid4vpClientID    string
	participantDID    string
}

func NewAuth(db *sqlx.DB, presentations authdb.PresentationAttemptRepo, cfg AuthConfig) (genauth.Service, error) {
	if cfg.Hydra == nil {
		return nil, fmt.Errorf("hydra client is required")
	}

	if cfg.Trust == nil {
		return nil, fmt.Errorf("oid4vp trust config is required")
	}

	if strings.TrimSpace(cfg.PublicAPIBase) == "" {
		return nil, fmt.Errorf("public API base URL is required")
	}

	if cfg.DCQLQuery == nil {
		return nil, fmt.Errorf("oid4vp DCQL query is required")
	}

	if strings.TrimSpace(cfg.ParticipantDID) == "" {
		return nil, fmt.Errorf("participant DID is required")
	}

	if cfg.PIDDCQLQuery == nil {
		return nil, fmt.Errorf("oid4vp PID DCQL query is required")
	}

	if db != nil {
		oid4vp.ConfigurePresentationAuditRecorder(&authaudit.Recorder{DB: db})
	}

	oid4vpStateTTL := cfg.OID4VPStateTTL
	if oid4vpStateTTL <= 0 {
		oid4vpStateTTL = defaultOID4VPStateTTL
	}

	return &authSvc{
		hydra:             cfg.Hydra,
		trust:             cfg.Trust,
		dcqlQuery:         cfg.DCQLQuery,
		pidDCQLQuery:      cfg.PIDDCQLQuery,
		logoutRedirectURI: cfg.LogoutRedirectURI,
		uiBasePath:        cfg.UIPath,
		publicAPIBase:     cfg.PublicAPIBase,
		oid4vpStateTTL:    oid4vpStateTTL,
		presentations:     presentations,
		requestSigner:     cfg.RequestSigner,
		oid4vpClientID:    cfg.OID4VPClientID,
		participantDID:    cfg.ParticipantDID,
	}, nil
}

func (s *authSvc) Callback(ctx context.Context, p *genauth.CallbackPayload) (*genauth.CallbackResult, error) {
	log.Printf(ctx, "auth.callback")
	defer ClearOAuthStateCookie(ctx)

	if oauthErr := oauthCallbackError(p); oauthErr != "" {
		clearAuthSessionCookies(ctx)
		log.Printf(ctx, "auth.callback oauth error: %s", oauthErr)
		return &genauth.CallbackResult{Location: oauthErrorRedirectLocation(s.uiBasePath, p)}, nil
	}

	code := ""
	if p.Code != nil {
		code = strings.TrimSpace(*p.Code)
	}

	if code == "" {
		return nil, goa.PermanentError("bad_request", "missing authorization code")
	}

	returnedState := ""
	if p.State != nil {
		returnedState = *p.State
	}

	if err := validateOAuthState(ctx, returnedState); err != nil {
		return nil, goa.PermanentError("unauthorized", "invalid oauth state: %v", err)
	}

	tokenResp, err := s.hydra.ExchangeCode(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("token exchange failed: %w", err)
	}

	if strings.TrimSpace(tokenResp.RefreshToken) == "" {
		return nil, goa.PermanentError("unauthorized", "token response missing refresh_token")
	}

	if strings.TrimSpace(tokenResp.IDToken) == "" {
		return nil, goa.PermanentError("unauthorized", "token response missing id_token (openid scope required)")
	}

	SetRefreshTokenInContext(ctx, tokenResp.RefreshToken)
	SetIDTokenCookie(ctx, tokenResp.IDToken)

	return &genauth.CallbackResult{
		Location: s.uiBasePath + "auth/success",
	}, nil
}

func (s *authSvc) Refresh(ctx context.Context) (*genauth.RefreshResult, error) {
	log.Printf(ctx, "auth.refresh")

	r, ok := HTTPRequestFromContext(ctx)
	if !ok {
		return nil, goa.PermanentError("unauthorized", "missing HTTP request in context")
	}

	cookie, err := r.Cookie("refresh_token")
	if err != nil {
		return nil, goa.PermanentError("unauthorized", "missing or invalid refresh token")
	}

	tokenResp, err := s.hydra.RefreshAccessToken(ctx, cookie.Value)
	if err != nil {
		ClearRefreshTokenCookie(ctx)
		return nil, goa.PermanentError("unauthorized", "token refresh failed: %v", err)
	}

	if strings.TrimSpace(tokenResp.RefreshToken) != "" {
		SetRefreshTokenInContext(ctx, tokenResp.RefreshToken)
	}

	if strings.TrimSpace(tokenResp.IDToken) != "" {
		SetIDTokenCookie(ctx, tokenResp.IDToken)
	}

	return &genauth.RefreshResult{
		AccessToken: tokenResp.AccessToken,
		TokenType:   tokenResp.TokenType,
		ExpiresIn:   tokenResp.ExpiresIn,
	}, nil
}

func (s *authSvc) Logout(ctx context.Context) (*genauth.LogoutResult, error) {
	log.Printf(ctx, "auth.logout")

	if strings.TrimSpace(s.logoutRedirectURI) == "" {
		return nil, goa.PermanentError("unauthorized", "HYDRA_POST_LOGOUT_REDIRECT_URI is not configured")
	}

	idTokenHint := readIDTokenCookie(ctx)
	if idTokenHint == "" {
		return nil, goa.PermanentError("unauthorized", "missing id_token session cookie")
	}

	if r, ok := HTTPRequestFromContext(ctx); ok {
		if cookie, err := r.Cookie("refresh_token"); err == nil && cookie.Value != "" {
			_ = s.hydra.RevokeRefreshToken(ctx, cookie.Value)
		}
	}

	endSessionEndpoint, err := s.hydra.EndSessionURL(ctx)
	if err != nil {
		return nil, fmt.Errorf("hydra end session endpoint: %w", err)
	}

	params := url.Values{}
	params.Set("client_id", s.hydra.ClientID())
	params.Set("id_token_hint", idTokenHint)
	params.Set("post_logout_redirect_uri", s.logoutRedirectURI)
	logoutURL := endSessionEndpoint + "?" + params.Encode()

	ClearRefreshTokenCookie(ctx)
	ClearIDTokenCookie(ctx)
	return &genauth.LogoutResult{LogoutURL: logoutURL}, nil
}

func (s *authSvc) LogoutComplete(ctx context.Context) (*genauth.LogoutCompleteResult, error) {
	log.Printf(ctx, "auth.logout-complete")
	ClearRefreshTokenCookie(ctx)
	ClearIDTokenCookie(ctx)
	return &genauth.LogoutCompleteResult{Location: s.uiBasePath}, nil
}

func newOAuthState() (string, error) {
	buf := make([]byte, oauthStateSizeBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func oauthCallbackError(p *genauth.CallbackPayload) string {
	if p.Error == nil {
		return ""
	}
	return strings.TrimSpace(*p.Error)
}

func oauthErrorRedirectLocation(uiBasePath string, p *genauth.CallbackPayload) string {
	loc, err := url.Parse(uiBasePath)
	if err != nil {
		return uiBasePath
	}

	q := loc.Query()
	if errCode := oauthCallbackError(p); errCode != "" {
		q.Set("auth_error", errCode)
	}

	if p.ErrorDescription != nil {
		if desc := strings.TrimSpace(*p.ErrorDescription); desc != "" {
			q.Set("auth_error_description", desc)
		}
	}

	loc.RawQuery = q.Encode()

	return loc.String()
}

func clearAuthSessionCookies(ctx context.Context) {
	ClearRefreshTokenCookie(ctx)
	ClearIDTokenCookie(ctx)
}

func validateOAuthState(ctx context.Context, returnedState string) error {
	if returnedState == "" {
		return fmt.Errorf("missing state")
	}

	expected, err := ReadOAuthStateCookie(ctx)
	if err != nil {
		return err
	}

	if subtle.ConstantTimeCompare([]byte(expected), []byte(returnedState)) == 1 {
		return nil
	}

	return fmt.Errorf("state mismatch")
}
