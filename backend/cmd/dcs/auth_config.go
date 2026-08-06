package main

import (
	"context"
	"crypto/x509"
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
	"digital-contracting-service/internal/pdfgeneration/provenance"
	"digital-contracting-service/internal/service"
)

// issuerX5ChainPathEnv points at the PEM certificate chain that publishes this
// deployment's own signing key — the chain the hsm-provision job issues for the
// C2PA claim signature (c2pa-x5chain.pem), bound to the same PKCS#11 key. It is
// what the status list this deployment serves carries in its x5c header, and
// what its own trust anchor is read out of.
const issuerX5ChainPathEnv = "DCS_ISSUER_X5CHAIN_PATH"

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

	// The two CA trust lists an x5c chain is verified against (ADR-35).
	//
	// PoA covers parties: a counterparty's Power of Attorney, admitted by its
	// chain alone, and our own login issuers — the same certificates seen from
	// this side, since the PoA a holder obtains at login here is what travels in
	// the signed PDF for the counterparty to verify. PID is separate so that a
	// CA attesting persons cannot speak for a party, or a party's CA attest a
	// person. Neither falls back to the other; a list left unset has no anchors,
	// and every x5c credential for it is refused rather than trusted off its own
	// embedded leaf cert (sdjwt.verificationKeyFromX5C).
	anchorPaths := map[oid4vp.Purpose]string{
		oid4vp.PurposePeer: "OID4VP_X5C_TRUST_ANCHORS_POA_PATH",
		oid4vp.PurposePID:  "OID4VP_X5C_TRUST_ANCHORS_PID_PATH",
	}
	for purpose, env := range anchorPaths {
		path := strings.TrimSpace(os.Getenv(env))
		if path == "" {
			continue
		}
		anchors, err := oid4vp.LoadX5CTrustAnchors(path)
		if err != nil {
			return service.AuthConfig{}, fmt.Errorf("oid4vp configuration error: %s: %w", env, err)
		}
		trustCfg.SetX5CTrustRoots(purpose, anchors)
	}

	// This deployment issues credentials of its own — the contract lifecycle
	// credential and the signing summary — and therefore serves and signs their
	// status list (ADR-34). It also verifies that list, through exactly the path
	// it verifies anyone else's: chain against the configured anchors, leaf
	// against the issuer the token names. So the anchor of its own chain has to
	// be among those anchors, and it cannot be a committed one — the
	// provisioning job mints it per install — so it is read back out of the
	// chain the deployment signs with. Without this every verification of our
	// own provenance credential would report the revocation state as unknown.
	//
	// It lands on the PoA list: what a counterparty reads from us is our own
	// provenance, issued under the same CA that issues our login issuer.
	if chainPath := strings.TrimSpace(os.Getenv(issuerX5ChainPathEnv)); chainPath != "" {
		root, err := provenance.StatusListRoot(chainPath)
		if err != nil {
			return service.AuthConfig{}, fmt.Errorf("dcs configuration error: %s: %w", issuerX5ChainPathEnv, err)
		}
		trustCfg.SetX5CTrustRoots(oid4vp.PurposePeer, []*x509.Certificate{root})
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
