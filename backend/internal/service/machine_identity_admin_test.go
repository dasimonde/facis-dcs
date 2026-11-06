package service

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"io"
	"strings"
	"testing"

	contractworkflowengine "digital-contracting-service/gen/contract_workflow_engine"
	"digital-contracting-service/internal/auth/datatype/eventtype"
	authevents "digital-contracting-service/internal/auth/event"
	"digital-contracting-service/internal/contractworkflowengine/db"
	"digital-contracting-service/internal/middleware"

	"github.com/jmoiron/sqlx"
)

// auditOutbox is a database that keeps every outbox INSERT event.Create sends;
// the repositories are stubs, so no other statement reaches it.
type auditOutbox struct{ rows [][]driver.Value }

func (o *auditOutbox) Connect(context.Context) (driver.Conn, error) { return o, nil }
func (o *auditOutbox) Driver() driver.Driver                        { return nil }
func (o *auditOutbox) Prepare(string) (driver.Stmt, error)          { return o, nil }
func (o *auditOutbox) Close() error                                 { return nil }
func (o *auditOutbox) Begin() (driver.Tx, error)                    { return o, nil }
func (o *auditOutbox) NumInput() int                                { return -1 }
func (o *auditOutbox) Query([]driver.Value) (driver.Rows, error)    { return nil, io.EOF }
func (o *auditOutbox) Commit() error                                { return nil }
func (o *auditOutbox) Rollback() error                              { return nil }
func (o *auditOutbox) Exec(args []driver.Value) (driver.Result, error) {
	o.rows = append(o.rows, args)
	return driver.RowsAffected(1), nil
}

func (o *auditOutbox) events(t *testing.T) []authevents.MachineIdentityChangedEvent {
	t.Helper()
	out := make([]authevents.MachineIdentityChangedEvent, 0, len(o.rows))
	for _, row := range o.rows {
		if !strings.HasPrefix(row[1].(string), "MACHINE_IDENTITY_") {
			continue
		}
		var evt authevents.MachineIdentityChangedEvent
		if err := json.Unmarshal(row[2].([]byte), &evt); err != nil {
			t.Fatal(err)
		}
		evt.Change = eventtype.EventType(row[1].(string))
		out = append(out, evt)
	}
	return out
}

const adminDID = "did:example:administrator"

func machineIdentityAdmin(t *testing.T) (*contractWorkflowEnginesrvc, *auditOutbox, *stubHydraAdmin, context.Context) {
	t.Helper()
	outbox := &auditOutbox{}
	raw := sql.OpenDB(outbox)
	t.Cleanup(func() { _ = raw.Close() })
	admin := &stubHydraAdmin{}
	svc := &contractWorkflowEnginesrvc{
		DB:                sqlx.NewDb(raw, "postgres"),
		DIDDocument:       *jadesTestDIDDocument(t, "dcs-a.localhost%3A18080"),
		MachineIdentities: newIdentityRegistry(),
		HydraAdmin:        admin,
	}
	ctx := middleware.InjectAuthContext(context.Background(), []string{"Sys. Administrator"}, adminDID, thisDeployment)
	return svc, outbox, admin, ctx
}

// NFR-BR-07: a machine credential is issued only to this deployment's own
// participant and only with system roles. A refused request provisions nothing.
func TestMachineCredentialsOnlyForThisParticipantWithSystemRoles(t *testing.T) {
	svc, _, admin, ctx := machineIdentityAdmin(t)

	for name, req := range map[string]*contractworkflowengine.MachineIdentityCreateRequest{
		"another participant": {Name: "ERP", ParticipantDid: "did:web:other.example", Roles: []string{"Sys. Contract Creator"}},
		"a human role":        {Name: "ERP", ParticipantDid: thisDeployment, Roles: []string{"Sys. Administrator"}},
	} {
		if _, err := svc.CreateMachineIdentity(ctx, req); err == nil {
			t.Fatalf("a credential was issued for %s", name)
		}
	}
	if len(admin.created) != 0 {
		t.Fatalf("a refused request provisioned OAuth2 clients: %v", admin.created)
	}

	created, err := svc.CreateMachineIdentity(ctx, &contractworkflowengine.MachineIdentityCreateRequest{
		Name: "ERP", ParticipantDid: thisDeployment, Roles: []string{"Sys. Contract Creator"},
	})
	if err != nil {
		t.Fatalf("a credential for this participant with a system role was refused: %v", err)
	}

	if _, err := svc.UpdateMachineIdentity(ctx, &contractworkflowengine.MachineIdentityUpdateRequest{
		ID: created.Identity.ID, Name: "ERP", ParticipantDid: "did:web:other.example", Roles: []string{"Sys. Contract Creator"}, Enabled: true,
	}); err == nil {
		t.Fatal("an update moved the credential to another participant")
	}
	if _, err := svc.UpdateMachineIdentity(ctx, &contractworkflowengine.MachineIdentityUpdateRequest{
		ID: created.Identity.ID, Name: "ERP", ParticipantDid: thisDeployment, Roles: []string{"Sys. Administrator"}, Enabled: true,
	}); err == nil {
		t.Fatal("an update gave the credential a human role")
	}
}

// FR-UC-09-2: every administrator change to a machine identity is in the audit
// trail, with the identity it changed and the administrator who changed it.
func TestMachineIdentityChangesAreAudited(t *testing.T) {
	svc, outbox, _, ctx := machineIdentityAdmin(t)

	created, err := svc.CreateMachineIdentity(ctx, &contractworkflowengine.MachineIdentityCreateRequest{
		Name: "ERP", ParticipantDid: thisDeployment, Roles: []string{"Sys. Contract Creator"},
	})
	if err != nil {
		t.Fatal(err)
	}
	id := created.Identity.ID
	if _, err := svc.UpdateMachineIdentity(ctx, &contractworkflowengine.MachineIdentityUpdateRequest{
		ID: id, Name: "ERP", ParticipantDid: thisDeployment, Roles: []string{"Sys. Contract Manager"}, Enabled: false,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RotateMachineIdentitySecret(ctx, &contractworkflowengine.MachineIdentityRotateRequest{ID: id}); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteMachineIdentity(ctx, &contractworkflowengine.MachineIdentityDeleteRequest{ID: id}); err != nil {
		t.Fatal(err)
	}

	events := outbox.events(t)
	want := []eventtype.EventType{
		eventtype.MachineIdentityCreated,
		eventtype.MachineIdentityUpdated,
		eventtype.MachineIdentitySecretRotated,
		eventtype.MachineIdentityDeleted,
	}
	if len(events) != len(want) {
		t.Fatalf("expected %d audit events, got %+v", len(want), events)
	}
	for i, evt := range events {
		if evt.Change != want[i] {
			t.Fatalf("event %d is %s, want %s", i, evt.Change, want[i])
		}
		if evt.OAuthClientID != created.Credential.ClientID || evt.ActorDID != adminDID || evt.ActorParticipantDID != thisDeployment {
			t.Fatalf("event %s does not name the identity and the administrator: %+v", evt.Change, evt)
		}
	}
	if updated := events[1]; updated.Enabled || len(updated.Roles) != 1 || updated.Roles[0] != "Sys. Contract Manager" {
		t.Fatalf("the update event does not record the identity after the change: %+v", updated)
	}
}

// The credential of a contract target is a machine identity too: its issue,
// rotation and removal with the target are audited the same way.
func TestContractTargetCredentialChangesAreAudited(t *testing.T) {
	svc, outbox, _, ctx := machineIdentityAdmin(t)
	svc.TargetRepo = &stubTargetRegistry{target: &db.ContractTarget{ID: "6f1a", Name: "Runtime target"}}

	for range 2 {
		if _, err := svc.RotateContractTargetSecret(ctx, &contractworkflowengine.ContractTargetRotateRequest{ID: "6f1a"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.DeleteContractTarget(ctx, &contractworkflowengine.ContractTargetDeleteRequest{ID: "6f1a"}); err != nil {
		t.Fatal(err)
	}

	events := outbox.events(t)
	want := []eventtype.EventType{
		eventtype.MachineIdentityCreated,
		eventtype.MachineIdentitySecretRotated,
		eventtype.MachineIdentityDeleted,
	}
	if len(events) != len(want) {
		t.Fatalf("expected %d audit events, got %+v", len(want), events)
	}
	for i, evt := range events {
		if evt.Change != want[i] || evt.OAuthClientID != "dcs-target-6f1a" || evt.ActorDID != adminDID {
			t.Fatalf("event %d is %+v, want %s of dcs-target-6f1a by %s", i, evt, want[i], adminDID)
		}
	}
}
