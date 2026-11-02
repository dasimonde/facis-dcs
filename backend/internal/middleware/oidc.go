package middleware

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"os"
	"slices"
	"strings"

	"digital-contracting-service/internal/auth/machineidentity"
	"digital-contracting-service/internal/base/datatype/userrole"

	"github.com/coreos/go-oidc/v3/oidc"
)

// HydraJWTConfig holds OIDC provider configuration.
type HydraJWTConfig struct {
	PublicIssuerURL   string
	InternalIssuerURL string
	// Example: "dcs-client". Hydra JWT access tokens use the client_id claim (RFC 9068).
	ClientID string
	// SystemClients resolves the machine identities of the SRS System User
	// classes (SRS §2.4 Table 5): integrated platforms and orchestration layers
	// that reach DCS over its API instead of through a browser. Nil accepts no
	// machine caller at all.
	SystemClients MachineIdentityResolver
}

// MachineIdentityResolver looks up a registered machine caller by the client_id
// its access token carries. It is satisfied by the machineidentity registry;
// the interface keeps this package from depending on storage.
type MachineIdentityResolver interface {
	FindByClientID(ctx context.Context, clientID string) (*machineidentity.Identity, error)
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
	provider        *oidc.Provider
	verifier        *oidc.IDTokenVerifier
	expiredVerifier *oidc.IDTokenVerifier
	config          HydraJWTConfig
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

	skipSignature, err := algNoneAllowed()
	if err != nil {
		return nil, err
	}
	// Skip audience check — client binding is validated in ValidateToken via client_id (and aud).
	// The token signature and issuer are still fully validated.
	oidcConfig := &oidc.Config{
		ClientID:                   config.ClientID,
		SkipClientIDCheck:          true,
		InsecureSkipSignatureCheck: skipSignature,
	}
	var meta struct {
		JWKSURL string   `json:"jwks_uri"`
		Algs    []string `json:"id_token_signing_alg_values_supported"`
	}
	if err := provider.Claims(&meta); err != nil {
		return nil, fmt.Errorf("read the OIDC provider metadata: %w", err)
	}
	// The metadata names the JWKS under the public issuer URL. The backend
	// fetches it where it fetched the discovery document: the public URL need
	// not be reachable from inside the cluster.
	jwksURL := meta.JWKSURL
	if discoveryURL != publicIssuer && strings.HasPrefix(jwksURL, publicIssuer+"/") {
		jwksURL = discoveryURL + strings.TrimPrefix(jwksURL, publicIssuer)
	}
	var keys oidc.KeySet = oidc.NewRemoteKeySet(ctx, jwksURL)
	oidcConfig.SupportedSigningAlgs = meta.Algs
	if len(oidcConfig.SupportedSigningAlgs) == 0 {
		oidcConfig.SupportedSigningAlgs = []string{oidc.RS256}
	}
	devKey, err := devAccessTokenKey()
	if err != nil {
		return nil, err
	}
	if devKey != nil {
		keys = devKeySet{
			dev:      &oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{devKey}},
			provider: keys,
		}
		oidcConfig.SupportedSigningAlgs = append(slices.Clone(oidcConfig.SupportedSigningAlgs), oidc.ES256)
	}
	newVerifier := func(config *oidc.Config) *oidc.IDTokenVerifier {
		return oidc.NewVerifier(publicIssuer, keys, config)
	}
	verifier := newVerifier(oidcConfig)
	expiredConfig := *oidcConfig
	expiredConfig.SkipExpiryCheck = true

	return &HydraJWTValidator{
		provider:        provider,
		verifier:        verifier,
		expiredVerifier: newVerifier(&expiredConfig),
		config:          config,
	}, nil
}

// devAccessTokenKey reads DCS_DEV_ACCESS_TOKEN_KEY, a PEM-encoded ES256 public
// key. A development or test stack accepts access tokens signed with it beside
// the provider's keys: the BDD stack signs the expired tokens of its negative
// credential scenarios with it, so it runs with the signature check on. Only a
// stack that declares itself one with DCS_ALLOW_DEV_TRUST=true may set it.
func devAccessTokenKey() (crypto.PublicKey, error) {
	raw := strings.TrimSpace(os.Getenv("DCS_DEV_ACCESS_TOKEN_KEY"))
	if raw == "" {
		return nil, nil
	}
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("DCS_ALLOW_DEV_TRUST")), "true") {
		return nil, fmt.Errorf("DCS_DEV_ACCESS_TOKEN_KEY admits access tokens signed with a development key; it is refused unless DCS_ALLOW_DEV_TRUST=true declares a development or test stack")
	}
	block, _ := pem.Decode([]byte(raw))
	if block == nil {
		return nil, fmt.Errorf("DCS_DEV_ACCESS_TOKEN_KEY is not a PEM public key")
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("DCS_DEV_ACCESS_TOKEN_KEY: %w", err)
	}
	if _, ok := key.(*ecdsa.PublicKey); !ok {
		return nil, fmt.Errorf("DCS_DEV_ACCESS_TOKEN_KEY is not an ECDSA public key")
	}
	log.Printf("WARNING: DCS_DEV_ACCESS_TOKEN_KEY: access tokens signed with a development key are accepted. Development and test stacks only.")
	return key, nil
}

// devKeySet verifies a token with the development key, else with the
// provider's keys.
type devKeySet struct {
	dev      oidc.KeySet
	provider oidc.KeySet
}

func (k devKeySet) VerifySignature(ctx context.Context, jwt string) ([]byte, error) {
	if payload, err := k.dev.VerifySignature(ctx, jwt); err == nil {
		return payload, nil
	}
	return k.provider.VerifySignature(ctx, jwt)
}

// algNoneAllowed reads JWT_ALG_NONE_SUPPORTED, which turns off the token
// signature check. Only a stack that declares itself a development or test
// stack with DCS_ALLOW_DEV_TRUST=true may set it (DCS-NFR-SEC-03).
func algNoneAllowed() (bool, error) {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("JWT_ALG_NONE_SUPPORTED")), "true") {
		return false, nil
	}
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("DCS_ALLOW_DEV_TRUST")), "true") {
		return false, fmt.Errorf("JWT_ALG_NONE_SUPPORTED=true turns off the access token signature check; it is refused unless DCS_ALLOW_DEV_TRUST=true declares a development or test stack")
	}
	log.Printf("WARNING: JWT_ALG_NONE_SUPPORTED=true: access token signatures are NOT checked; any caller can forge any role. Development and test stacks only.")
	return true, nil
}

// ExpiredTokenSubject returns the subject of a token whose signature and
// issuer are valid but whose lifetime ended, or "" for any other token. It
// identifies the caller of a refused expired token in the attempt log.
func (v *HydraJWTValidator) ExpiredTokenSubject(ctx context.Context, token string) string {
	idToken, err := v.expiredVerifier.Verify(ctx, token)
	if err != nil {
		return ""
	}
	return idToken.Subject
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

	// A machine caller's token is a client-credentials token: Hydra signed it, so
	// the client secret was presented, but no OID4VP ceremony ran and there are
	// no ext claims to read. Its authority is looked up by client_id in the
	// registry, never taken from the token.
	if system, ok, err := v.systemClientFor(ctx, claims); err != nil {
		return nil, err
	} else if ok {
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

// systemClientFor resolves a token against the machine identity registry. Only
// an exact client_id/audience match counts — a machine caller's rights are not
// something a token can ask for.
//
// A disabled entry resolves to no caller rather than to an unauthorised error:
// the token then falls through to the human path and is refused there, so
// disabling an integration revokes it without waiting for its secret to expire.
func (v *HydraJWTValidator) systemClientFor(ctx context.Context, claims Claims) (SystemClient, bool, error) {
	if v.config.SystemClients == nil {
		return SystemClient{}, false, nil
	}

	for _, candidate := range clientIDCandidates(claims) {
		identity, err := v.config.SystemClients.FindByClientID(ctx, candidate)
		if err != nil {
			return SystemClient{}, false, fmt.Errorf("could not resolve the calling machine identity: %w", err)
		}
		if identity == nil || !identity.Enabled {
			continue
		}

		roles, err := identity.Roles()
		if err != nil {
			return SystemClient{}, false, err
		}
		return SystemClient{
			ClientID:       identity.OAuthClientID,
			ParticipantDID: identity.ParticipantDID,
			Roles:          roles,
		}, true, nil
	}
	return SystemClient{}, false, nil
}

// clientIDCandidates lists the identifiers a token could name itself by. Hydra
// puts the client on client_id (RFC 9068), but a token that only carries an
// audience is still resolvable, which is what the human path relies on too.
func clientIDCandidates(claims Claims) []string {
	candidates := []string{}
	if id := strings.TrimSpace(claims.ClientID); id != "" {
		candidates = append(candidates, id)
	}
	switch aud := claims.Audience.(type) {
	case string:
		if trimmed := strings.TrimSpace(aud); trimmed != "" {
			candidates = append(candidates, trimmed)
		}
	case []interface{}:
		for _, item := range aud {
			if audience, ok := item.(string); ok && strings.TrimSpace(audience) != "" {
				candidates = append(candidates, strings.TrimSpace(audience))
			}
		}
	}
	return candidates
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
