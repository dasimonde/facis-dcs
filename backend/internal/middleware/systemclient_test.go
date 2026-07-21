package middleware

import "testing"

// A system client's authority comes from deployment configuration, never from
// the token: a client-credentials token carries no ext claims, and anything it
// did carry must not widen what the caller may do.
func TestSystemClientRolesComeFromConfigurationNotTheToken(t *testing.T) {
	validator := &HydraJWTValidator{config: HydraJWTConfig{
		ClientID: "dcs-client",
		SystemClients: []SystemClient{{
			ClientID:       "dcs-orce-system",
			ParticipantDID: "did:web:orce.example",
			Roles:          []string{"Auditor"},
		}},
	}}

	claims := Claims{
		ClientID: "dcs-orce-system",
		Ext:      map[string]interface{}{"roles": []interface{}{"Sys. Administrator"}},
	}
	system, ok := validator.systemClientFor(claims)
	if !ok {
		t.Fatal("configured system client was not recognised")
	}
	if len(system.Roles) != 1 || system.Roles[0] != "Auditor" {
		t.Fatalf("roles came from the token instead of the configuration: %v", system.Roles)
	}
	if system.ParticipantDID != "did:web:orce.example" {
		t.Fatalf("wrong participant attribution: %q", system.ParticipantDID)
	}
}

// An unconfigured client is not a system user, however well-formed its token.
func TestUnknownClientIsNotASystemUser(t *testing.T) {
	validator := &HydraJWTValidator{config: HydraJWTConfig{
		ClientID:      "dcs-client",
		SystemClients: []SystemClient{{ClientID: "dcs-orce-system", ParticipantDID: "did:web:orce.example", Roles: []string{"Auditor"}}},
	}}

	if _, ok := validator.systemClientFor(Claims{ClientID: "some-other-client"}); ok {
		t.Fatal("an unconfigured client was accepted as a system user")
	}
	if _, ok := validator.systemClientFor(Claims{Audience: "dcs-orce-system"}); !ok {
		t.Fatal("a system client identified by audience was not recognised")
	}
}

// With nothing configured, no client-credentials token gets in.
func TestNoSystemClientsConfiguredRejectsAll(t *testing.T) {
	validator := &HydraJWTValidator{config: HydraJWTConfig{ClientID: "dcs-client"}}
	if _, ok := validator.systemClientFor(Claims{ClientID: "dcs-orce-system"}); ok {
		t.Fatal("a system client was accepted although none is configured")
	}
}
