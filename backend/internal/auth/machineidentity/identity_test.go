package machineidentity

import (
	"testing"
	"time"
)

// The row an issued target credential produces has to be the row the
// DCS_SYSTEM_CLIENTS seed produces for a target declared in deployment
// configuration (cmd/dcs/machine_identity_seed.go): named after the client it
// authenticates as, attributed to the deployment, enabled, holding exactly the
// Contract Target System role. Where the two drift, only one of the two ways to
// register a target ever authorises a callback.
func TestContractTargetIdentityMatchesTheSeededRowShape(t *testing.T) {
	issued := time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC)
	identity, err := ContractTargetCredential{
		ClientID:       "dcs-target-6f1a",
		TargetName:     "Runtime target",
		ParticipantDID: "did:web:dcs-a.localhost%3A18080",
		IssuedBy:       "did:web:operator.example",
		IssuedAt:       issued,
	}.Identity()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The seed names the identity after its client, and machine_identities.name
	// is unique — a generated name taken from the target would collide.
	if identity.Name != "dcs-target-6f1a" || identity.OAuthClientID != "dcs-target-6f1a" {
		t.Fatalf("row is not keyed on the client it authenticates as: name=%q client=%q", identity.Name, identity.OAuthClientID)
	}
	if identity.ParticipantDID != "did:web:dcs-a.localhost%3A18080" {
		t.Fatalf("wrong attribution: %q", identity.ParticipantDID)
	}
	if !identity.Enabled {
		t.Fatal("a freshly issued credential is disabled and resolves to no caller")
	}
	roles, err := identity.Roles()
	if err != nil {
		t.Fatalf("roles are unreadable: %v", err)
	}
	if len(roles) != 1 || roles[0] != "Contract Target System" {
		t.Fatalf("expected exactly the Contract Target System role, got %v", roles)
	}
	if identity.CreatedBy != "did:web:operator.example" {
		t.Fatalf("the issuer was not recorded: %q", identity.CreatedBy)
	}
	if identity.SecretIssuedAt == nil || !identity.SecretIssuedAt.Equal(issued) {
		t.Fatalf("the issue date was not recorded: %v", identity.SecretIssuedAt)
	}
	if identity.Description == nil || *identity.Description == "" {
		t.Fatal("nothing says which target the client belongs to")
	}
}

// A row with no client to match, or nobody to attribute its callbacks to,
// cannot authorise anything — it is refused where it is written rather than
// where it first fails to resolve.
func TestContractTargetIdentityNeedsAClientAndADID(t *testing.T) {
	if _, err := (ContractTargetCredential{ParticipantDID: "did:web:dcs.example"}).Identity(); err == nil {
		t.Fatal("an identity with no OAuth2 client was accepted")
	}
	if _, err := (ContractTargetCredential{ClientID: "dcs-target-6f1a"}).Identity(); err == nil {
		t.Fatal("an identity with no participant DID was accepted")
	}
}

func TestRolesRoundTrip(t *testing.T) {
	encoded, err := EncodeRoles([]string{"Sys. Contract Creator", "Sys. Auditor"})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	roles, err := Identity{RolesJSON: encoded}.Roles()
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(roles) != 2 || roles[0] != "Sys. Contract Creator" || roles[1] != "Sys. Auditor" {
		t.Fatalf("roles did not survive the round trip: %v", roles)
	}
}

// An identity with no roles could authenticate and then do nothing, with
// nothing to say why, so it is refused when it is written rather than when it
// first calls.
func TestEncodeRolesRefusesAnEmptyList(t *testing.T) {
	if _, err := EncodeRoles(nil); err == nil {
		t.Fatal("an identity with no roles was accepted")
	}
}

func TestRolesReportsUnreadableStorage(t *testing.T) {
	if _, err := (Identity{Name: "broken", RolesJSON: "not json"}).Roles(); err == nil {
		t.Fatal("an unreadable role list was accepted")
	}
}

// A row written before any roles existed decodes as none rather than failing,
// so the caller is refused by having no authority instead of by an error.
func TestEmptyRolesJSONDecodesAsNone(t *testing.T) {
	roles, err := (Identity{RolesJSON: "  "}).Roles()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(roles) != 0 {
		t.Fatalf("expected no roles, got %v", roles)
	}
}
