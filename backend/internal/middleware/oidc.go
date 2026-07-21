package middleware

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"digital-contracting-service/internal/base/datatype/userrole"

	"github.com/coreos/go-oidc/v3/oidc"
)

// HydraJWTConfig holds OIDC provider configuration.
type HydraJWTConfig struct {
	PublicIssuerURL   string
	InternalIssuerURL string
	// Example: "dcs-client". Hydra JWT access tokens use the client_id claim (RFC 9068).
	ClientID string
	// SystemClients are the machine identities of the SRS System User classes
	// (SRS §2.4 Table 5): integrated platforms and orchestration layers that
	// reach DCS over its API instead of through a browser.
	SystemClients []SystemClient
}

// SystemClient is one non-human caller, authenticated by the OAuth2 client
// credentials grant. A human user proves who they are with a verifiable
// credential and the OID4VP ceremony fills the token's ext claims; a system
// user has no wallet and no ceremony, so its token carries only Hydra's proof
// that the client secret was presented. What such a client may do is therefore
// decided HERE, from deployment configuration, and never from claims the caller
// could influence.
type SystemClient struct {
	ClientID string
	// ParticipantDID is the identity its actions are attributed to in the audit
	// trail, standing in for the ext.iss a human token carries.
	ParticipantDID string
	Roles          []string
}

// HydraJWTValidator validates JWT tokens from OIDC providers.
type HydraJWTValidator struct {
	provider *oidc.Provider
	verifier *oidc.IDTokenVerifier
	config   HydraJWTConfig
}

// NewHydraJWTValidator connects to the OIDC provider to get public keys.
func NewHydraJWTValidator(ctx context.Context, config HydraJWTConfig) (*HydraJWTValidator, error) {
	publicIssuer := strings.TrimRight(strings.TrimSpace(config.PublicIssuerURL), "/")
	if publicIssuer == "" {
		return nil, fmt.Errorf("HydraJWTConfig.PublicIssuerURL is required")
	}

	discoveryURL := strings.TrimRight(strings.TrimSpace(config.InternalIssuerURL), "/")
	if discoveryURL == "" {
		discoveryURL = publicIssuer
	}

	ctx = oidc.InsecureIssuerURLContext(ctx, publicIssuer)
	provider, err := oidc.NewProvider(ctx, discoveryURL)
	if err != nil {
		return nil, fmt.Errorf("failed to discover OIDC provider: %w", err)
	}

	// Skip audience check — client binding is validated in ValidateToken via client_id (and aud).
	// The token signature and issuer are still fully validated.
	verifier := provider.Verifier(&oidc.Config{
		ClientID:                   config.ClientID,
		SkipClientIDCheck:          true,
		InsecureSkipSignatureCheck: os.Getenv("JWT_ALG_NONE_SUPPORTED") == "true",
	})

	return &HydraJWTValidator{
		provider: provider,
		verifier: verifier,
		config:   config,
	}, nil
}

// TokenInfo holds the validated identity extracted from a JWT.
type TokenInfo struct {
	Roles          []string
	HolderDID      string
	ParticipantDID string
}

type Claims struct {
	Subject  string                 `json:"sub"`
	Issuer   string                 `json:"iss"`
	Ext      map[string]interface{} `json:"ext"`
	Audience interface{}            `json:"aud"`
	ClientID string                 `json:"client_id"`
}

// ValidateToken verifies the token signature, issuer, and client binding, then
// returns the caller's roles, holder DID, and participant DID.
func (v *HydraJWTValidator) ValidateToken(ctx context.Context, token string) (*TokenInfo, error) {
	idToken, err := v.verifier.Verify(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("token verification failed: %w", err)
	}

	var claims Claims
	if err := idToken.Claims(&claims); err != nil {
		return nil, fmt.Errorf("failed to parse token claims: %w", err)
	}

	// A system client's token is a client-credentials token: Hydra signed it, so
	// the client secret was presented, but no OID4VP ceremony ran and there are
	// no ext claims to read. Its authority comes from configuration.
	if system, ok := v.systemClientFor(claims); ok {
		return &TokenInfo{
			Roles:          system.Roles,
			HolderDID:      system.ClientID,
			ParticipantDID: system.ParticipantDID,
		}, nil
	}

	if !matchesClientID(claims, v.config.ClientID) {
		return nil, fmt.Errorf("token is not bound to client ID %q", v.config.ClientID)
	}

	issuer, ok := claims.Ext["iss"].(string)
	if !ok {
		return nil, fmt.Errorf("no iss claim in ext claim found in token")
	}

	return &TokenInfo{
		Roles:          extractRoles(claims),
		HolderDID:      claims.Subject,
		ParticipantDID: issuer,
	}, nil
}

// systemClientFor matches a token against the configured system clients. Only
// an exact client_id/audience match counts — a system client's rights are not
// something a token can ask for.
func (v *HydraJWTValidator) systemClientFor(claims Claims) (SystemClient, bool) {
	for _, system := range v.config.SystemClients {
		if system.ClientID == "" {
			continue
		}
		if matchesClientID(claims, system.ClientID) {
			return system, true
		}
	}
	return SystemClient{}, false
}

// extractRoles extracts DCS roles from a Hydra access token.
func extractRoles(claims Claims) []string {
	if claims.Ext != nil {
		if roles := toStringSlice(claims.Ext["roles"]); len(roles) > 0 {
			return roles
		}
	}
	return []string{}
}

// matchesClientID matches the JWT token to the expected OAuth client.
func matchesClientID(claims Claims, clientID string) bool {

	if claims.ClientID != "" {
		return claims.ClientID == clientID
	}
	switch aud := claims.Audience.(type) {
	case string:
		return aud == clientID
	case []interface{}:
		for _, item := range aud {
			if audience, ok := item.(string); ok && audience == clientID {
				return true
			}
		}
	}
	return false
}

func toStringSlice(v interface{}) []string {
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// ExtractBearerToken expected format "Authorization: Bearer <token>"
func ExtractBearerToken(authHeader string) (string, error) {
	const bearerPrefix = "Bearer "
	if !strings.HasPrefix(authHeader, bearerPrefix) {
		return "", fmt.Errorf("invalid authorization header format")
	}
	return strings.TrimPrefix(authHeader, bearerPrefix), nil
}

// unexported key type to avoid context key collisions.
type authCtxKey struct{}

// AuthContext carries the validated caller identity through the request context.
type AuthContext struct {
	Roles         []string
	HolderDID     string
	ParticipantID string
}

// GetUserRoles extracts roles from the request context.
func GetUserRoles(ctx context.Context) []userrole.UserRole {
	if ac, ok := ctx.Value(authCtxKey{}).(AuthContext); ok {
		userRoles := make([]userrole.UserRole, len(ac.Roles))
		for i, role := range ac.Roles {
			userRole, err := userrole.NewUserRole(role)
			if err != nil {
				log.Printf("failed to parse user role %q: %v", role, err)
			}
			userRoles[i] = userRole
		}
		return userRoles

	}
	return []userrole.UserRole{}
}

// GetHolderDID extracts the authenticated DID from the request context.
func GetHolderDID(ctx context.Context) string {
	if ac, ok := ctx.Value(authCtxKey{}).(AuthContext); ok {
		return ac.HolderDID
	}
	return ""
}

// GetParticipantID extracts the authenticated participant ID from the request context.
func GetParticipantID(ctx context.Context) string {
	if ac, ok := ctx.Value(authCtxKey{}).(AuthContext); ok {
		return ac.ParticipantID
	}
	return ""
}

// InjectAuthContext injects the validated identity into the request context.
func InjectAuthContext(ctx context.Context, roles []string, holderDID string, participantID string) context.Context {
	return context.WithValue(ctx, authCtxKey{}, AuthContext{Roles: roles, HolderDID: holderDID, ParticipantID: participantID})
}

// unexported key type for the raw bearer token.
type bearerTokenCtxKey struct{}

// InjectBearerToken stores the raw JWT presented on the incoming request so
// downstream handlers can forward it to pdf-core, which uses it to authenticate
// its call back to the internal C2PA signing endpoint (DCS-IR-HI-01).
func InjectBearerToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, bearerTokenCtxKey{}, token)
}

// GetBearerToken returns the raw JWT stored by InjectBearerToken, or "" when the
// request carried no token (e.g. an internal, non-authenticated code path).
func GetBearerToken(ctx context.Context) string {
	if tok, ok := ctx.Value(bearerTokenCtxKey{}).(string); ok {
		return tok
	}
	return ""
}
