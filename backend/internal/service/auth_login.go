package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	genauth "digital-contracting-service/gen/auth"
	authdb "digital-contracting-service/internal/auth/db"
	"digital-contracting-service/internal/auth/oid4vp"
	oid4vprequest "digital-contracting-service/internal/auth/oid4vp/request"

	goa "goa.design/goa/v3/pkg"
)

const defaultOID4VPStateTTL = 5 * time.Minute

func (s *authSvc) LoginRenew(ctx context.Context, p *genauth.LoginRenewPayload) (*genauth.LoginRenewResult, error) {
	state := strings.TrimSpace(p.State)
	if state == "" {
		return nil, goa.PermanentError("bad_request", "state is required")
	}

	attempt, err := s.presentations.FindByPresentationState(ctx, state)
	if err != nil {
		return nil, err
	}

	if attempt == nil {
		return nil, goa.PermanentError("not_found", "unknown presentation state")
	}

	switch attempt.Status {
	case authdb.PresentationPending, authdb.PresentationExpired:
	default:
		return nil, goa.PermanentError("bad_request", "presentation state cannot be renewed")
	}

	ttl := s.oid4vpStateTTL
	expiresAt := time.Now().UTC().Add(ttl)

	if err := s.presentations.RenewPending(ctx, state, expiresAt); err != nil {
		if authdb.IsPresentationNotPending(err) {
			return nil, goa.PermanentError("bad_request", "presentation state cannot be renewed")
		}
		return nil, err
	}

	login, err := s.buildLoginResult(ctx, state, attempt.Nonce, int(ttl.Seconds()))
	if err != nil {
		return nil, err
	}

	return loginResultToRenew(login), nil
}

func loginResultToRenew(in *genauth.LoginResult) *genauth.LoginRenewResult {
	return &genauth.LoginRenewResult{
		RequestURI:      in.RequestURI,
		PresentationURL: in.PresentationURL,
		State:           in.State,
		AuthorizeURL:    in.AuthorizeURL,
		ExpiresIn:       in.ExpiresIn,
	}
}

func (s *authSvc) Login(ctx context.Context) (*genauth.LoginResult, error) {
	ttl := s.oid4vpStateTTL
	presentationState, err := newOAuthState()

	if err != nil {
		return nil, err
	}

	nonce, err := newOAuthState()

	if err != nil {
		return nil, err
	}

	SetOAuthStateCookie(ctx, presentationState)

	expiresAt := time.Now().UTC().Add(ttl)

	err = s.presentations.CreatePending(ctx, authdb.PresentationAttempt{
		PresentationState: presentationState,
		Nonce:             nonce,
		ExpiresAt:         expiresAt,
	})
	if err != nil {
		return nil, err
	}

	return s.buildLoginResult(ctx, presentationState, nonce, int(ttl.Seconds()))
}

func (s *authSvc) buildLoginResult(ctx context.Context, presentationState, nonce string, expiresIn int) (*genauth.LoginResult, error) {
	authorizeURL, err := s.hydra.AuthorizeURL(ctx, presentationState)

	if err != nil {
		return nil, goa.PermanentError("service_unavailable", "hydra authorize url: %v", err)
	}

	requestURI := strings.TrimRight(s.publicAPIBase, "/") + "/auth/presentation/request/" + url.PathEscape(presentationState)

	return &genauth.LoginResult{
		RequestURI:      requestURI,
		PresentationURL: buildOpenID4VPPresentationURI(s.oid4vpClientID, requestURI),
		State:           presentationState,
		AuthorizeURL:    authorizeURL,
		ExpiresIn:       expiresIn,
	}, nil
}

func (s *authSvc) Consent(ctx context.Context, p *genauth.ConsentPayload) (*genauth.ConsentResult, error) {
	challenge := strings.TrimSpace(p.ConsentChallenge)

	if challenge == "" {
		return nil, goa.PermanentError("bad_request", "consent_challenge is required")
	}

	redirectTo, err := s.hydra.AcceptConsent(ctx, challenge)
	if err != nil {
		return nil, goa.PermanentError("unauthorized", "hydra consent: %v", err)
	}

	return &genauth.ConsentResult{Location: redirectTo}, nil
}

func (s *authSvc) LoginChallenge(ctx context.Context, p *genauth.LoginChallengePayload) error {
	challenge := strings.TrimSpace(p.LoginChallenge)
	if challenge == "" {
		return goa.PermanentError("bad_request", "login_challenge is required")
	}

	attempt, err := s.loadPresentationAttempt(ctx, p.State)
	if err != nil {
		return err
	}

	if attempt.Status != authdb.PresentationPending {
		return goa.PermanentError("bad_request", "presentation state is not pending")
	}

	if attempt.HydraLoginChallenge != nil && strings.TrimSpace(*attempt.HydraLoginChallenge) == challenge {
		return nil
	}

	if err := s.presentations.SetHydraLoginChallenge(ctx, p.State, challenge); err != nil {
		if authdb.IsPresentationNotPending(err) {
			return goa.PermanentError("bad_request", "presentation state is not pending")
		}
		return err
	}
	return nil
}

func (s *authSvc) PresentationRequest(ctx context.Context, p *genauth.PresentationRequestPayload) (io.ReadCloser, error) {
	attempt, err := s.loadPresentationAttempt(ctx, p.State)
	if err != nil {
		return nil, err
	}

	walletNonce := strings.TrimSpace(pointerString(p.WalletNonce))

	responseURI := strings.TrimRight(s.publicAPIBase, "/") + "/auth/presentation/callback"
	jwt, err := oid4vprequest.BuildJWT(s.requestSigner, oid4vprequest.Params{
		ClientID:    s.oid4vpClientID,
		ResponseURI: responseURI,
		State:       attempt.PresentationState,
		Nonce:       attempt.Nonce,
		WalletNonce: walletNonce,
		ExpiresAt:   attempt.ExpiresAt,
		DCQLQuery:   s.dcqlQuery,
	})

	if err != nil {
		return nil, goa.PermanentError("internal", "authorization request: %v", err)
	}

	return io.NopCloser(bytes.NewReader([]byte(jwt))), nil
}

func (s *authSvc) PresentationCallback(ctx context.Context, p *genauth.PresentationCallbackPayload) (*genauth.PresentationCallbackResult, error) {
	attempt, err := s.loadPresentationAttempt(ctx, p.State)

	if err != nil {
		return nil, err
	}

	if attempt.Status != authdb.PresentationPending {
		return nil, goa.PermanentError("bad_request", "presentation state is not pending")
	}

	walletError := strings.TrimSpace(pointerString(p.Error))
	if walletError != "" {
		desc := strings.TrimSpace(pointerString(p.ErrorDescription))
		message := walletError
		if desc != "" {
			message += ": " + desc
		}

		_ = s.presentations.MarkFailed(ctx, attempt.PresentationState, message)
		oid4vp.RecordPresentationAudit(ctx, oid4vp.PresentationAuditEvent{
			PresentationState: attempt.PresentationState,
			Success:           false,
			ErrorMessage:      message,
		})
		return &genauth.PresentationCallbackResult{}, nil
	}

	if attempt.HydraLoginChallenge == nil || strings.TrimSpace(*attempt.HydraLoginChallenge) == "" {
		return nil, goa.PermanentError("bad_request", "missing hydra login challenge; complete browser authorize first")
	}

	vpToken := ""
	if p.VpToken != nil {
		vpToken = *p.VpToken
	}

	queryIDs, err := credentialQueryIDsFromDCQL(s.dcqlQuery)
	if err != nil {
		return nil, goa.PermanentError("bad_request", "invalid dcql_query: %v", err)
	}

	presentation, err := extractSinglePresentation(vpToken, queryIDs...)
	if err != nil {
		return nil, goa.PermanentError("bad_request", "invalid vp_token: %v", err)
	}

	verified, err := oid4vp.NewVerifier(s.trust, oid4vp.PurposeLogin).Verify(presentation, oid4vp.PresentationContext{
		Nonce:    attempt.Nonce,
		ClientID: s.oid4vpClientID,
	})

	if err != nil {
		_ = s.presentations.MarkFailed(ctx, attempt.PresentationState, err.Error())
		oid4vp.RecordPresentationAudit(ctx, oid4vp.PresentationAuditEvent{
			PresentationState: attempt.PresentationState,
			Success:           false,
			ErrorMessage:      err.Error(),
		})
		return nil, goa.PermanentError("unauthorized", "vp verification failed: %v", err)
	}

	grantedRoles := verified.GrantedRoles

	redirectTo, err := s.hydra.AcceptLoginAndConsent(
		ctx,
		*attempt.HydraLoginChallenge,
		verified.SubjectDID,
		verified.ParticipantDID,
		grantedRoles,
	)

	if err != nil {
		_ = s.presentations.MarkFailed(ctx, attempt.PresentationState, err.Error())
		oid4vp.RecordPresentationAudit(ctx, oid4vp.PresentationAuditEvent{
			PresentationState: attempt.PresentationState,
			Success:           false,
			SubjectDID:        verified.SubjectDID,
			ParticipantDID:    verified.ParticipantDID,
			Roles:             grantedRoles,
			ErrorMessage:      err.Error(),
		})
		return nil, goa.PermanentError("unauthorized", "hydra login: %v", err)
	}

	rolesJSON, _ := json.Marshal(grantedRoles)

	if err := s.presentations.MarkComplete(ctx, attempt.PresentationState, verified.RawClaims, verified.SubjectDID, verified.ParticipantDID, rolesJSON, redirectTo); err != nil {
		return nil, err
	}

	oid4vp.RecordPresentationAudit(ctx, oid4vp.PresentationAuditEvent{
		PresentationState: attempt.PresentationState,
		Success:           true,
		SubjectDID:        verified.SubjectDID,
		ParticipantDID:    verified.ParticipantDID,
		Roles:             grantedRoles,
	})

	return &genauth.PresentationCallbackResult{}, nil
}

func pointerString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// credentialQueryIDsFromDCQL returns every credential query id the request
// offered the wallet. A query asks for one credential under more than one
// format identifier, and the wallet answers under the id of whichever it
// matched, so the answer is looked for under all of them.
func credentialQueryIDsFromDCQL(dcqlQuery any) ([]string, error) {
	query, ok := dcqlQuery.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("dcql query must be a JSON object")
	}

	rawCredentials, ok := query["credentials"]
	if !ok {
		return nil, fmt.Errorf("missing credentials")
	}

	credentials, ok := rawCredentials.([]any)
	if !ok || len(credentials) == 0 {
		return nil, fmt.Errorf("credentials must be a non-empty array")
	}

	ids := make([]string, 0, len(credentials))
	for i, raw := range credentials {
		entry, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("credentials[%d] must be an object", i)
		}
		id, _ := entry["id"].(string)
		id = strings.TrimSpace(id)
		if id == "" {
			return nil, fmt.Errorf("credentials[%d].id is required", i)
		}
		ids = append(ids, id)
	}

	return ids, nil
}

func extractSinglePresentation(rawVPToken string, queryIDs ...string) (string, error) {
	rawVPToken = strings.TrimSpace(rawVPToken)
	if rawVPToken == "" {
		return "", fmt.Errorf("vp_token is required")
	}
	if len(queryIDs) == 0 {
		return "", fmt.Errorf("no credential query id to read the vp_token under")
	}

	var tokenByQuery map[string]json.RawMessage
	if err := json.Unmarshal([]byte(rawVPToken), &tokenByQuery); err != nil {
		return "", fmt.Errorf("vp_token must be a JSON object")
	}

	for _, queryID := range queryIDs {
		rawPresentations, ok := tokenByQuery[queryID]
		if !ok {
			continue
		}

		var presentations []string
		if err := json.Unmarshal(rawPresentations, &presentations); err != nil {
			return "", fmt.Errorf("vp_token[%q] must be an array of strings", queryID)
		}

		if len(presentations) != 1 {
			return "", fmt.Errorf("vp_token[%q] must contain exactly one presentation", queryID)
		}

		presentation := strings.TrimSpace(presentations[0])
		if presentation == "" {
			return "", fmt.Errorf("vp_token[%q][0] must be a non-empty string", queryID)
		}

		return presentation, nil
	}

	return "", fmt.Errorf("missing vp_token entry for query ids %v", queryIDs)
}

func (s *authSvc) LoginStatus(ctx context.Context, p *genauth.PresentationStatePayload) (*genauth.PresentationStatusResult, error) {
	attempt, err := s.presentations.FindByPresentationState(ctx, p.State)
	if err != nil {
		return nil, err
	}

	if attempt == nil {
		return nil, goa.PermanentError("not_found", "unknown presentation state")
	}

	now := time.Now().UTC()
	expiresIn := int(attempt.ExpiresAt.Sub(now).Seconds())
	if expiresIn < 0 {
		expiresIn = 0
	}

	status := string(attempt.Status)
	if attempt.Status == authdb.PresentationPending && now.After(attempt.ExpiresAt) {
		if err := s.presentations.MarkExpired(ctx, attempt.PresentationState); err == nil {
			status = string(authdb.PresentationExpired)
			expiresIn = 0
		}
	}

	out := &genauth.PresentationStatusResult{
		State:     attempt.PresentationState,
		Status:    status,
		ExpiresIn: expiresIn,
	}
	if attempt.RedirectURI != nil && status == string(authdb.PresentationComplete) {
		out.RedirectURI = attempt.RedirectURI
	}

	if attempt.ErrorMessage != nil && status == string(authdb.PresentationFailed) {

		out.ErrorMessage = attempt.ErrorMessage
	}

	return out, nil
}

func (s *authSvc) loadPresentationAttempt(ctx context.Context, presentationState string) (*authdb.PresentationAttempt, error) {
	attempt, err := s.presentations.FindByPresentationState(ctx, presentationState)

	if err != nil {
		return nil, err
	}

	if attempt == nil {
		return nil, goa.PermanentError("not_found", "unknown presentation state")
	}

	if attempt.Status == authdb.PresentationPending && time.Now().UTC().After(attempt.ExpiresAt) {
		_ = s.presentations.MarkExpired(ctx, presentationState)
		return nil, goa.PermanentError("bad_request", "presentation state expired")
	}

	return attempt, nil
}

// buildOpenID4VPPresentationURI returns a cross-device wallet deep link (OpenID4VP request-by-reference).
func buildOpenID4VPPresentationURI(clientID, httpsRequestURI string) string {
	q := url.Values{}
	q.Set("client_id", clientID)
	q.Set("request_uri", httpsRequestURI)
	q.Set("request_uri_method", "post")

	return "openid4vp://?" + q.Encode()
}
