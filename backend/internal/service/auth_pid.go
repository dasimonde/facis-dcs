package service

import (
	"bytes"
	"context"
	"encoding/json"
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

func (s *authSvc) PidPresentation(ctx context.Context) (*genauth.PidPresentationResult, error) {
	ttl := s.oid4vpStateTTL
	presentationState, err := newOAuthState()
	if err != nil {
		return nil, err
	}

	nonce, err := newOAuthState()
	if err != nil {
		return nil, err
	}

	expiresAt := time.Now().UTC().Add(ttl)
	err = s.presentations.CreatePending(ctx, authdb.PresentationAttempt{
		PresentationState: presentationState,
		Nonce:             nonce,
		ExpiresAt:         expiresAt,
	})

	if err != nil {
		return nil, err
	}

	return s.buildPidPresentationResult(presentationState, int(ttl.Seconds()))
}

func (s *authSvc) PidPresentationRenew(ctx context.Context, p *genauth.PidPresentationRenewPayload) (*genauth.PidPresentationResult, error) {
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

	return s.buildPidPresentationResult(state, int(ttl.Seconds()))
}

func (s *authSvc) buildPidPresentationResult(presentationState string, expiresIn int) (*genauth.PidPresentationResult, error) {
	requestURI := strings.TrimRight(s.publicAPIBase, "/") + "/auth/pid/presentation/request/" + url.PathEscape(presentationState)

	return &genauth.PidPresentationResult{
		PresentationURL: buildOpenID4VPPresentationURI(s.oid4vpClientID, requestURI),
		State:           presentationState,
		ExpiresIn:       expiresIn,
	}, nil
}

func (s *authSvc) PidPresentationRequest(ctx context.Context, p *genauth.PidPresentationRequestPayload) (io.ReadCloser, error) {
	attempt, err := s.loadPresentationAttempt(ctx, p.State)

	if err != nil {
		return nil, err
	}

	walletNonce := strings.TrimSpace(pointerString(p.WalletNonce))
	responseURI := strings.TrimRight(s.publicAPIBase, "/") + "/auth/pid/presentation/callback"
	jwt, err := oid4vprequest.BuildJWT(s.requestSigner, oid4vprequest.Params{
		ClientID:    s.oid4vpClientID,
		ResponseURI: responseURI,
		State:       attempt.PresentationState,
		Nonce:       attempt.Nonce,
		WalletNonce: walletNonce,
		ExpiresAt:   attempt.ExpiresAt,
		DCQLQuery:   s.pidDCQLQuery,
	})

	if err != nil {
		return nil, goa.PermanentError("internal", "authorization request: %v", err)
	}

	return io.NopCloser(bytes.NewReader([]byte(jwt))), nil
}

func (s *authSvc) PidPresentationCallback(ctx context.Context, p *genauth.PresentationCallbackPayload) error {
	attempt, err := s.loadPresentationAttempt(ctx, p.State)
	if err != nil {
		return err
	}

	if attempt.Status != authdb.PresentationPending {
		return goa.PermanentError("bad_request", "presentation state is not pending")
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
		return nil
	}

	vpToken := ""
	if p.VpToken != nil {
		vpToken = *p.VpToken
	}

	queryIDs, err := credentialQueryIDsFromDCQL(s.pidDCQLQuery)
	if err != nil {
		return goa.PermanentError("bad_request", "invalid pid dcql_query: %v", err)
	}

	presentation, err := extractSinglePresentation(vpToken, queryIDs...)
	if err != nil {
		return goa.PermanentError("bad_request", "invalid vp_token: %v", err)
	}

	verified, err := oid4vp.NewVerifier(s.trust, oid4vp.PurposePID).VerifyPID(presentation, oid4vp.PresentationContext{
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
		return goa.PermanentError("unauthorized", "vp verification failed: %v", err)
	}

	rolesJSON, _ := json.Marshal([]string{})
	if err := s.presentations.MarkComplete(
		ctx,
		attempt.PresentationState,
		verified.RawClaims,
		verified.SubjectDID,
		"",
		rolesJSON,
		"",
	); err != nil {
		return err
	}

	oid4vp.RecordPresentationAudit(ctx, oid4vp.PresentationAuditEvent{
		PresentationState: attempt.PresentationState,
		Success:           true,
		SubjectDID:        verified.SubjectDID,
	})

	return nil
}

func (s *authSvc) PidPresentationStatus(ctx context.Context, p *genauth.PresentationStatePayload) (*genauth.PresentationStatusResult, error) {
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

	if attempt.ErrorMessage != nil && status == string(authdb.PresentationFailed) {
		out.ErrorMessage = attempt.ErrorMessage
	}

	return out, nil
}
