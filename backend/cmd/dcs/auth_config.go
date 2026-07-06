package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"digital-contracting-service/internal/auth/hydra"
	"digital-contracting-service/internal/auth/oid4vp"
	oid4vprequest "digital-contracting-service/internal/auth/oid4vp/request"
	"digital-contracting-service/internal/pathutil"
	"digital-contracting-service/internal/service"
)

func loadAuthConfig(ctx context.Context) (service.AuthConfig, error) {
	publicIssuerURL := strings.TrimRight(strings.TrimSpace(os.Getenv("HYDRA_PUBLIC_ISSUER_URL")), "/")
	if publicIssuerURL == "" {
		return service.AuthConfig{}, fmt.Errorf("hydra configuration missing: HYDRA_PUBLIC_ISSUER_URL must be set")
	}

	internalIssuerURL := strings.TrimRight(strings.TrimSpace(os.Getenv("HYDRA_INTERNAL_ISSUER_URL")), "/")

	clientID := strings.TrimSpace(os.Getenv("HYDRA_CLIENT_ID"))
	if clientID == "" {
		return service.AuthConfig{}, fmt.Errorf("hydra configuration missing: HYDRA_CLIENT_ID must be set")
	}

	clientSecret := strings.TrimSpace(os.Getenv("HYDRA_CLIENT_SECRET"))
	if clientSecret == "" {
		return service.AuthConfig{}, fmt.Errorf("hydra configuration missing: HYDRA_CLIENT_SECRET must be set")
	}

	redirectURI := strings.TrimSpace(os.Getenv("HYDRA_REDIRECT_URI"))
	if redirectURI == "" {
		return service.AuthConfig{}, fmt.Errorf("hydra configuration missing: HYDRA_REDIRECT_URI must be set")
	}

	adminURL := strings.TrimRight(strings.TrimSpace(os.Getenv("HYDRA_ADMIN_URL")), "/")
	if adminURL == "" {
		return service.AuthConfig{}, fmt.Errorf("hydra configuration missing: HYDRA_ADMIN_URL must be set")
	}

	trustDataPath := strings.TrimSpace(os.Getenv("OID4VP_TRUST_DATA_PATH"))
	if trustDataPath == "" {
		return service.AuthConfig{}, fmt.Errorf("oid4vp configuration missing: OID4VP_TRUST_DATA_PATH must be set")
	}

	trustCfg, err := oid4vp.LoadTrustConfig(trustDataPath)
	if err != nil {
		return service.AuthConfig{}, fmt.Errorf("oid4vp configuration error: %w", err)
	}

	skipStatusListJWS := false
	if v := strings.TrimSpace(os.Getenv("OID4VP_STATUSLIST_SKIP_JWS_VERIFY")); strings.EqualFold(v, "true") {
		skipStatusListJWS = true
	}
	oid4vp.ConfigureStatusListJWTVerification(trustCfg, skipStatusListJWS)

	skipStatusListIfUnavailable := false
	if v := strings.TrimSpace(os.Getenv("OID4VP_STATUSLIST_SKIP_IF_UNAVAILABLE")); strings.EqualFold(v, "true") {
		skipStatusListIfUnavailable = true
	}
	oid4vp.ConfigureStatusListSkipIfUnavailable(skipStatusListIfUnavailable)

	dcqlQuery, err := oid4vp.LoadDCQLQuery(os.Getenv("OID4VP_DCQL_QUERY"))
	if err != nil {
		return service.AuthConfig{}, fmt.Errorf("oid4vp configuration error: %w", err)
	}

	var requestSigner oid4vprequest.Signer
	vaultAddr := strings.TrimRight(strings.TrimSpace(os.Getenv("VAULT_ADDR")), "/")

	if vaultAddr != "" {
		signer, signerErr := oid4vprequest.NewVaultTransitSigner(
			vaultAddr,
			os.Getenv("VAULT_TOKEN"),
			os.Getenv("OID4VP_VERIFIER_SIGNING_VAULT_MOUNT"),
			os.Getenv("OID4VP_VERIFIER_SIGNING_VAULT_KEY"),
		)
		if signerErr != nil {
			return service.AuthConfig{}, fmt.Errorf("oid4vp request signer configuration error: %w", signerErr)
		} else {
			requestSigner = signer
		}
	}

	publicAPIBase := strings.TrimRight(strings.TrimSpace(os.Getenv("DCS_PUBLIC_BASE_URL")), "/")
	if publicAPIBase == "" {
		return service.AuthConfig{}, fmt.Errorf("dcs configuration missing: DCS_PUBLIC_BASE_URL must be set")
	}

	logoutRedirectURI := strings.TrimSpace(os.Getenv("HYDRA_POST_LOGOUT_REDIRECT_URI"))
	uiPath := pathutil.NormalizePath(os.Getenv("DCS_UI_PATH"), "/ui/", true)

	var oid4vpStateTTL time.Duration

	if v := strings.TrimSpace(os.Getenv("OID4VP_STATE_TTL_SECONDS")); v != "" {
		secs, err := strconv.Atoi(v)
		if err != nil || secs <= 0 {
			return service.AuthConfig{}, fmt.Errorf("oid4vp configuration error: OID4VP_STATE_TTL_SECONDS must be a positive integer, got %q", v)
		}
		oid4vpStateTTL = time.Duration(secs) * time.Second
	}

	return service.AuthConfig{
		Hydra: hydra.New(hydra.Config{
			PublicIssuerURL:   publicIssuerURL,
			InternalIssuerURL: internalIssuerURL,
			ClientID:          clientID,
			ClientSecret:      clientSecret,
			RedirectURI:       redirectURI,
			AdminURL:          adminURL,
		}),
		Trust:             trustCfg,
		DCQLQuery:         dcqlQuery,
		RequestSigner:     requestSigner,
		PublicAPIBase:     publicAPIBase,
		LogoutRedirectURI: logoutRedirectURI,
		UIPath:            uiPath,
		OID4VPStateTTL:    oid4vpStateTTL,
	}, nil
}
