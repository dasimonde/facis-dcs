package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"digital-contracting-service/internal/auth/hydra"
	"digital-contracting-service/internal/auth/machineidentity"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/contractworkflowengine/db"

	"github.com/jmoiron/sqlx"
)

// The stubs below stand in for the two tables the credential endpoint writes.
// They ignore the transaction handle the way the callback authorization tests
// do: what is under test is which rows are written, not how they commit.

type stubTargetRegistry struct {
	target *db.ContractTarget
	// credentials records every SetCredential, so a rotation can be checked
	// against what the target row ends up naming.
	credentials []string
}

func (s *stubTargetRegistry) ReadTarget(context.Context, *sqlx.Tx, string) (*db.ContractTarget, error) {
	return s.target, nil
}

func (s *stubTargetRegistry) SetCredential(_ context.Context, _ *sqlx.Tx, _ string, clientID string, issuedAt time.Time) error {
	s.credentials = append(s.credentials, clientID)
	s.target.OAuthClientID = &clientID
	s.target.SecretIssuedAt = &issuedAt
	return nil
}

func (s *stubTargetRegistry) ListTargets(context.Context, *sqlx.Tx) ([]db.ContractTarget, error) {
	return nil, nil
}

func (s *stubTargetRegistry) CreateTarget(context.Context, *sqlx.Tx, db.ContractTarget) (*db.ContractTarget, error) {
	return nil, nil
}

func (s *stubTargetRegistry) UpdateTarget(context.Context, *sqlx.Tx, db.ContractTarget) (*db.ContractTarget, error) {
	return nil, nil
}

func (s *stubTargetRegistry) DeleteTarget(context.Context, *sqlx.Tx, string) error { return nil }

func (s *stubTargetRegistry) CountContractsDesignating(context.Context, *sqlx.Tx, string) (int, error) {
	return 0, nil
}

func (s *stubTargetRegistry) DesignateForContract(context.Context, *sqlx.Tx, string, *string) (bool, error) {
	return false, nil
}

// stubIdentityRegistry keys identities the way the real registry does — by the
// OAuth2 client — so a duplicate registration is visible as a second entry
// rather than hidden by a slice.
type stubIdentityRegistry struct {
	rows map[string]machineidentity.Identity
}

func newIdentityRegistry() *stubIdentityRegistry {
	return &stubIdentityRegistry{rows: map[string]machineidentity.Identity{}}
}

func (s *stubIdentityRegistry) UpsertTx(_ context.Context, _ *sqlx.Tx, data machineidentity.Identity) error {
	s.rows[data.OAuthClientID] = data
	return nil
}

func (s *stubIdentityRegistry) Upsert(_ context.Context, data machineidentity.Identity) error {
	s.rows[data.OAuthClientID] = data
	return nil
}

func (s *stubIdentityRegistry) FindByClientID(_ context.Context, clientID string) (*machineidentity.Identity, error) {
	if identity, ok := s.rows[clientID]; ok {
		return &identity, nil
	}
	return nil, nil
}

func (s *stubIdentityRegistry) List(context.Context) ([]machineidentity.Identity, error) {
	return nil, nil
}

func (s *stubIdentityRegistry) Read(context.Context, string) (*machineidentity.Identity, error) {
	return nil, nil
}

func (s *stubIdentityRegistry) Create(context.Context, machineidentity.Identity) (*machineidentity.Identity, error) {
	return nil, nil
}

func (s *stubIdentityRegistry) Update(context.Context, machineidentity.Identity) (*machineidentity.Identity, error) {
	return nil, nil
}

func (s *stubIdentityRegistry) Delete(context.Context, string) error { return nil }

func (s *stubIdentityRegistry) DeleteByClientIDTx(_ context.Context, _ *sqlx.Tx, clientID string) error {
	delete(s.rows, clientID)
	return nil
}

func (s *stubIdentityRegistry) TouchSecretIssuedAt(context.Context, string, time.Time) error {
	return nil
}

type stubHydraAdmin struct {
	created []string
	rotated []string
	deleted []string
}

func (s *stubHydraAdmin) CreateMachineClient(_ context.Context, clientID, _ string) (*hydra.OAuth2Client, error) {
	s.created = append(s.created, clientID)
	return &hydra.OAuth2Client{ClientID: clientID, Secret: "secret-" + clientID}, nil
}

func (s *stubHydraAdmin) RotateMachineClientSecret(_ context.Context, clientID string) (string, error) {
	s.rotated = append(s.rotated, clientID)
	return "rotated-" + clientID, nil
}

func (s *stubHydraAdmin) DeleteMachineClient(_ context.Context, clientID string) error {
	s.deleted = append(s.deleted, clientID)
	return nil
}

const thisDeployment = "did:web:dcs-a.localhost%3A18080"

func targetService(targets *stubTargetRegistry, identities *stubIdentityRegistry, admin *stubHydraAdmin) *contractWorkflowEnginesrvc {
	return &contractWorkflowEnginesrvc{
		TargetRepo:        targets,
		MachineIdentities: identities,
		HydraAdmin:        admin,
	}
}

// A credential issued through the API has to authorise the deployment callback,
// which means the registry must resolve its client to the Contract Target
// System role. Without the registry row the token authenticates and is then
// refused at the scope gate, and the deployment it acknowledges stays SIGNED.
func TestIssuedTargetCredentialResolvesToTheContractTargetRole(t *testing.T) {
	targets := &stubTargetRegistry{target: &db.ContractTarget{ID: "6f1a", Name: "Runtime target"}}
	identities := newIdentityRegistry()
	admin := &stubHydraAdmin{}

	issued, err := targetService(targets, identities, admin).
		issueTargetCredential(context.Background(), nil, "6f1a", thisDeployment)
	if err != nil {
		t.Fatalf("issuing the credential failed: %v", err)
	}
	if issued.clientID != "dcs-target-6f1a" {
		t.Fatalf("unexpected client id %q", issued.clientID)
	}

	identity, err := identities.FindByClientID(context.Background(), issued.clientID)
	if err != nil {
		t.Fatalf("registry lookup failed: %v", err)
	}
	if identity == nil {
		t.Fatal("the issued client resolves to no machine identity: its token carries no scope and the callback is refused")
	}
	if !identity.Enabled {
		t.Fatal("the issued identity is disabled and resolves to no caller")
	}
	roles, err := identity.Roles()
	if err != nil {
		t.Fatalf("stored roles are unreadable: %v", err)
	}
	if len(roles) != 1 || roles[0] != userrole.ContractTargetSystem.String() {
		t.Fatalf("expected exactly the Contract Target System role, got %v", roles)
	}
	if identity.ParticipantDID != thisDeployment {
		t.Fatalf("callbacks are attributed to %q, not to this deployment", identity.ParticipantDID)
	}
	if identity.SecretIssuedAt == nil {
		t.Fatal("the registry does not record that a secret was issued")
	}
}

// A rotation must leave the target with one usable identity: the same client it
// already authenticates as, refreshed — never a second row granting scope to a
// client that has been retired.
func TestRotationLeavesExactlyOneUsableTargetIdentity(t *testing.T) {
	targets := &stubTargetRegistry{target: &db.ContractTarget{ID: "6f1a", Name: "Runtime target"}}
	identities := newIdentityRegistry()
	admin := &stubHydraAdmin{}
	svc := targetService(targets, identities, admin)

	first, err := svc.issueTargetCredential(context.Background(), nil, "6f1a", thisDeployment)
	if err != nil {
		t.Fatalf("first issue failed: %v", err)
	}
	second, err := svc.issueTargetCredential(context.Background(), nil, "6f1a", thisDeployment)
	if err != nil {
		t.Fatalf("rotation failed: %v", err)
	}

	if second.clientID != first.clientID {
		t.Fatalf("rotation moved the target to client %q from %q, stranding the first", second.clientID, first.clientID)
	}
	if second.secret == first.secret {
		t.Fatal("rotation handed back the same secret")
	}
	if len(identities.rows) != 1 {
		t.Fatalf("expected one registered identity after a rotation, got %d: %v", len(identities.rows), identities.rows)
	}
	if len(admin.created) != 1 {
		t.Fatalf("expected the client to be provisioned once, got %v", admin.created)
	}
	if len(admin.rotated) != 1 || admin.rotated[0] != first.clientID {
		t.Fatalf("expected the existing client to be rotated, got %v", admin.rotated)
	}
}

// A target whose client comes from deployment configuration keeps it: the chart
// declares the client id on the target and the matching system client, and
// issuing a credential must refresh that same pair rather than provision a
// second client the callback would not be recognised as.
func TestSeededTargetKeepsItsConfiguredClient(t *testing.T) {
	seeded := "dcs-orce-target"
	targets := &stubTargetRegistry{target: &db.ContractTarget{
		ID:            "6f1a",
		Name:          "BDD Contract Target",
		OAuthClientID: &seeded,
	}}
	identities := newIdentityRegistry()
	admin := &stubHydraAdmin{}

	issued, err := targetService(targets, identities, admin).
		issueTargetCredential(context.Background(), nil, "6f1a", thisDeployment)
	if err != nil {
		t.Fatalf("issuing the credential failed: %v", err)
	}
	if issued.clientID != seeded {
		t.Fatalf("the configured client was replaced by %q", issued.clientID)
	}
	if len(admin.created) != 0 {
		t.Fatalf("a second client was provisioned for a configured target: %v", admin.created)
	}
	identity := identities.rows[seeded]
	if identity.Name != seeded {
		t.Fatalf("the registry row is named %q, not after the client it authenticates as", identity.Name)
	}
	roles, err := identity.Roles()
	if err != nil || len(roles) != 1 || roles[0] != userrole.ContractTargetSystem.String() {
		t.Fatalf("the seeded client's role was not preserved: %v (%v)", roles, err)
	}
	if identity.ParticipantDID != thisDeployment {
		t.Fatalf("attribution changed to %q", identity.ParticipantDID)
	}
}

// A target with no registry entry has no credential to issue, and must not be
// left with an OAuth2 client nothing accounts for.
func TestUnknownTargetIsRefused(t *testing.T) {
	identities := newIdentityRegistry()
	admin := &stubHydraAdmin{}

	_, err := targetService(&stubTargetRegistry{}, identities, admin).
		issueTargetCredential(context.Background(), nil, "missing", thisDeployment)
	if err == nil {
		t.Fatal("a credential was issued for a target that is not registered")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("the refusal does not name the target: %v", err)
	}
	if len(admin.created) != 0 || len(identities.rows) != 0 {
		t.Fatalf("something was provisioned for an unknown target: clients=%v identities=%v", admin.created, identities.rows)
	}
}
