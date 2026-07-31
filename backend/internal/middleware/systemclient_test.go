package middleware

import (
	"context"
	"encoding/json"
	"testing"

	"digital-contracting-service/internal/auth/machineidentity"
)

// stubRegistry resolves machine identities from memory, keyed the way the real
// registry keys them: by the OAuth2 client the token authenticated as.
type stubRegistry map[string]machineidentity.Identity

func (s stubRegistry) FindByClientID(_ context.Context, clientID string) (*machineidentity.Identity, error) {
	if identity, ok := s[clientID]; ok {
		return &identity, nil
	}
	return nil, nil
}

func registered(clientID, participantDID string, enabled bool, roles ...string) machineidentity.Identity {
	encoded, err := json.Marshal(roles)
	if err != nil {
		panic(err)
	}
	return machineidentity.Identity{
		OAuthClientID:  clientID,
		ParticipantDID: participantDID,
		RolesJSON:      string(encoded),
		Enabled:        enabled,
	}
}

// A machine caller's authority comes from the registry, never from the token: a
// client-credentials token carries no ext claims, and anything it did carry
// must not widen what the caller may do.
func TestSystemClientRolesComeFromTheRegistryNotTheToken(t *testing.T) {
	validator := &HydraJWTValidator{config: HydraJWTConfig{
		ClientID: "dcs-client",
		SystemClients: stubRegistry{
			"dcs-orce-system": registered("dcs-orce-system", "did:web:orce.example", true, "Auditor"),
		},
	}}

	claims := Claims{
		ClientID: "dcs-orce-system",
		Ext:      map[string]interface{}{"roles": []interface{}{"Sys. Administrator"}},
	}
	system, ok, err := validator.systemClientFor(context.Background(), claims)
	if err != nil {
		t.Fatalf("resolution failed: %v", err)
	}
	if !ok {
		t.Fatal("registered machine identity was not recognised")
	}
	if len(system.Roles) != 1 || system.Roles[0] != "Auditor" {
		t.Fatalf("roles came from the token instead of the registry: %v", system.Roles)
	}
	if system.ParticipantDID != "did:web:orce.example" {
		t.Fatalf("wrong participant attribution: %q", system.ParticipantDID)
	}
}

// An unregistered client is not a machine caller, however well-formed its token.
func TestUnknownClientIsNotASystemUser(t *testing.T) {
	validator := &HydraJWTValidator{config: HydraJWTConfig{
		ClientID: "dcs-client",
		SystemClients: stubRegistry{
			"dcs-orce-system": registered("dcs-orce-system", "did:web:orce.example", true, "Auditor"),
		},
	}}

	if _, ok, _ := validator.systemClientFor(context.Background(), Claims{ClientID: "some-other-client"}); ok {
		t.Fatal("an unregistered client was accepted as a machine caller")
	}
	if _, ok, _ := validator.systemClientFor(context.Background(), Claims{Audience: "dcs-orce-system"}); !ok {
		t.Fatal("a machine caller identified by audience was not recognised")
	}
}

// Disabling an identity revokes it at once, rather than leaving it usable until
// its secret happens to expire.
func TestDisabledIdentityIsRefused(t *testing.T) {
	validator := &HydraJWTValidator{config: HydraJWTConfig{
		ClientID: "dcs-client",
		SystemClients: stubRegistry{
			"dcs-orce-system": registered("dcs-orce-system", "did:web:orce.example", false, "Auditor"),
		},
	}}

	if _, ok, _ := validator.systemClientFor(context.Background(), Claims{ClientID: "dcs-orce-system"}); ok {
		t.Fatal("a disabled machine identity was still accepted")
	}
}

// The credential issued to a contract target through the API resolves here, at
// the same gate the declaratively seeded targets pass: this is where the
// Contract Target System scope its deployment callback needs comes from, and a
// client with no registry row gets none.
func TestIssuedContractTargetCredentialResolvesToItsRole(t *testing.T) {
	identity, err := machineidentity.ContractTargetCredential{
		ClientID:       "dcs-target-6f1a",
		TargetName:     "Runtime target",
		ParticipantDID: "did:web:dcs-a.localhost%3A18080",
	}.Identity()
	if err != nil {
		t.Fatalf("could not build the target identity: %v", err)
	}

	validator := &HydraJWTValidator{config: HydraJWTConfig{
		ClientID:      "dcs-client",
		SystemClients: stubRegistry{identity.OAuthClientID: identity},
	}}

	system, ok, err := validator.systemClientFor(context.Background(), Claims{ClientID: "dcs-target-6f1a"})
	if err != nil {
		t.Fatalf("resolution failed: %v", err)
	}
	if !ok {
		t.Fatal("an API-issued target credential was not recognised as a machine caller")
	}
	if len(system.Roles) != 1 || system.Roles[0] != "Contract Target System" {
		t.Fatalf("the target credential carries %v, not the Contract Target System role", system.Roles)
	}
	if system.ParticipantDID != "did:web:dcs-a.localhost%3A18080" {
		t.Fatalf("wrong participant attribution: %q", system.ParticipantDID)
	}
}

// With no registry wired at all, no client-credentials token gets in.
func TestNoRegistryRejectsAll(t *testing.T) {
	validator := &HydraJWTValidator{config: HydraJWTConfig{ClientID: "dcs-client"}}
	if _, ok, _ := validator.systemClientFor(context.Background(), Claims{ClientID: "dcs-orce-system"}); ok {
		t.Fatal("a machine caller was accepted although no registry is configured")
	}
}
