package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"digital-contracting-service/internal/auth/hydra"
	"digital-contracting-service/internal/auth/oid4vp"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/middleware"
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

	// Optional: a real EUDI-wallet-issued PID (or any x5c-bearing credential)
	// carries its issuer certificate this way, not a bare JWK. Unset in
	// environments that never present one (dev/BDD issue PID credentials via
	// the JWKS issuer path); an x5c credential presented with no trust
	// anchors configured is refused outright, never silently trusted off its
	// own embedded leaf cert (sdjwt.verificationKeyFromX5C).
	if x5cPath := strings.TrimSpace(os.Getenv("OID4VP_X5C_TRUST_ANCHORS_PATH")); x5cPath != "" {
		x5cRoots, err := oid4vp.LoadX5CTrustAnchors(x5cPath)
		if err != nil {
			return service.AuthConfig{}, fmt.Errorf("oid4vp configuration error: %w", err)
		}
		trustCfg.SetX5CTrustRoots(x5cRoots)
	}

	// Optional: the endpoint the `orce` mechanism delegates key resolution to.
	// It is what lets a deployment trust an issuer published through a registry
	// this build knows nothing about — the flow resolves the identifier and
	// answers with a JWKS. An issuer configured for `orce` without this set is
	// refused at first use with that reason named.
	if orceResolver := strings.TrimSpace(os.Getenv("OID4VP_ORCE_RESOLVER_URL")); orceResolver != "" {
		trustCfg.ORCEResolverURL = orceResolver
	}

	if err := oid4vp.ConfigureStatusListVerification(trustCfg); err != nil {
		return service.AuthConfig{}, fmt.Errorf("oid4vp configuration error: %w", err)
	}

	dcqlQuery, err := oid4vp.LoadDCQLQuery(os.Getenv("OID4VP_DCQL_QUERY"))
	if err != nil {
		return service.AuthConfig{}, fmt.Errorf("oid4vp configuration error: %w", err)
	}

	pidDCQLQuery, err := oid4vp.LoadPIDDCQLQuery(os.Getenv("OID4VP_PID_DCQL_QUERY"))
	if err != nil {
		return service.AuthConfig{}, fmt.Errorf("oid4vp configuration error: %w", err)
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
		PIDDCQLQuery:      pidDCQLQuery,
		PublicAPIBase:     publicAPIBase,
		LogoutRedirectURI: logoutRedirectURI,
		UIPath:            uiPath,
		OID4VPStateTTL:    oid4vpStateTTL,
	}, nil
}

// loadSystemClients reads the SRS System User clients (SRS §2.4 Table 5) from
// DCS_SYSTEM_CLIENTS, a JSON array of {client_id, participant_did, roles}.
// These are machine callers that authenticate with the OAuth2 client
// credentials grant, so their authority comes from configuration and not from
// token claims — a system client can present nothing that widens it.
//
// This is a seed, not the runtime source: entries are written into the machine
// identity registry at startup and resolved from there, so an operator can add
// or rotate a caller without a redeploy (ADR-27). Unset seeds nothing.
func loadSystemClients() ([]middleware.SystemClient, error) {
	raw := strings.TrimSpace(os.Getenv("DCS_SYSTEM_CLIENTS"))
	if raw == "" {
		return nil, nil
	}

	var configured []struct {
		ClientID       string   `json:"client_id"`
		ParticipantDID string   `json:"participant_did"`
		Roles          []string `json:"roles"`
	}
	if err := json.Unmarshal([]byte(raw), &configured); err != nil {
		return nil, fmt.Errorf("DCS_SYSTEM_CLIENTS is not a JSON array of {client_id, participant_did, roles}: %w", err)
	}

	clients := make([]middleware.SystemClient, 0, len(configured))
	for _, entry := range configured {
		clientID := strings.TrimSpace(entry.ClientID)
		if clientID == "" {
			return nil, fmt.Errorf("DCS_SYSTEM_CLIENTS: an entry has no client_id")
		}
		if strings.TrimSpace(entry.ParticipantDID) == "" {
			return nil, fmt.Errorf("DCS_SYSTEM_CLIENTS: client %q has no participant_did to attribute its actions to", clientID)
		}
		if len(entry.Roles) == 0 {
			return nil, fmt.Errorf("DCS_SYSTEM_CLIENTS: client %q has no roles", clientID)
		}
		for _, role := range entry.Roles {
			if !userrole.UserRole(role).IsValid() {
				return nil, fmt.Errorf("DCS_SYSTEM_CLIENTS: client %q has unknown role %q", clientID, role)
			}
		}
		clients = append(clients, middleware.SystemClient{
			ClientID:       clientID,
			ParticipantDID: strings.TrimSpace(entry.ParticipantDID),
			Roles:          entry.Roles,
		})
	}
	return clients, nil
}
