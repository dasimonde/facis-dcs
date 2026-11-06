package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	contractworkflowengine "digital-contracting-service/gen/contract_workflow_engine"
	"digital-contracting-service/internal/auth/machineidentity"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/contractworkflowengine/db/pg"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
	"digital-contracting-service/internal/middleware"
)

func configurationAdminPostgres(t *testing.T) (*contractWorkflowEnginesrvc, *sqlx.DB, *stubHydraAdmin, context.Context) {
	t.Helper()
	database := pgtest.Open(t)
	admin := &stubHydraAdmin{}
	svc := &contractWorkflowEnginesrvc{
		DB:                database,
		DIDDocument:       *jadesTestDIDDocument(t, "dcs-a.localhost%3A18080"),
		TargetRepo:        &pg.PostgresContractTargetRepo{},
		MachineIdentities: machineidentity.NewPostgresRepo(database),
		HydraAdmin:        admin,
	}
	ctx := middleware.InjectAuthContext(context.Background(), []string{"Sys. Administrator"}, adminDID, thisDeployment)
	return svc, database, admin, ctx
}

// refuseAuditOf makes every outbox write of eventType fail, as an audit write
// that fails would.
func refuseAuditOf(t *testing.T, database *sqlx.DB, eventType string) {
	t.Helper()
	_, err := database.Exec(`ALTER TABLE outbox_events ADD CONSTRAINT refuse_audit CHECK (event_type <> '` + eventType + `') NOT VALID`)
	require.NoError(t, err)
}

// FR-UC-09-2 (UC-IF-21): creating, changing, disabling and deleting a contract
// target system is in the audit trail with the settings before and after, the
// administrator and the roles they acted in.
func TestContractTargetChangesAreAuditedPostgres(t *testing.T) {
	svc, database, _, ctx := configurationAdminPostgres(t)

	created, err := svc.CreateContractTarget(ctx, &contractworkflowengine.ContractTargetCreateRequest{Name: "ERP", URL: "https://erp.example/deploy"})
	require.NoError(t, err)
	disabled := false
	_, err = svc.UpdateContractTarget(ctx, &contractworkflowengine.ContractTargetUpdateRequest{
		ID: created.ID, Name: "ERP", URL: "https://erp2.example/deploy", Enabled: &disabled,
	})
	require.NoError(t, err)
	require.NoError(t, svc.DeleteContractTarget(ctx, &contractworkflowengine.ContractTargetDeleteRequest{ID: created.ID}))

	var rows []struct {
		Component string `db:"component"`
		EventType string `db:"event_type"`
		EventData []byte `db:"event_data"`
		DID       string `db:"did"`
	}
	require.NoError(t, database.Select(&rows,
		`SELECT component, event_type, event_data, did FROM outbox_events WHERE event_type LIKE '%CONTRACT_TARGET' ORDER BY id`))
	require.Len(t, rows, 3)
	events := make([]contractevents.ContractTargetChangedEvent, len(rows))
	for i, row := range rows {
		require.Equal(t, "SYSTEM", row.Component)
		require.Equal(t, "contract-target:"+created.ID, row.DID)
		require.NoError(t, json.Unmarshal(row.EventData, &events[i]))
		require.Equal(t, adminDID, events[i].ActorDID)
		require.Equal(t, thisDeployment, events[i].ActorParticipantDID)
		require.Equal(t, userrole.UserRoles{userrole.SystemAdministrator}, events[i].UserRoles)
	}
	require.Equal(t, []string{"CREATE_CONTRACT_TARGET", "UPDATE_CONTRACT_TARGET", "DELETE_CONTRACT_TARGET"},
		[]string{rows[0].EventType, rows[1].EventType, rows[2].EventType})

	require.Nil(t, events[0].Before)
	require.Equal(t, "https://erp.example/deploy", events[0].After.URL)
	require.True(t, events[0].After.Enabled)

	require.Equal(t, "https://erp.example/deploy", events[1].Before.URL)
	require.True(t, events[1].Before.Enabled)
	require.Equal(t, "https://erp2.example/deploy", events[1].After.URL)
	require.False(t, events[1].After.Enabled)

	require.Equal(t, "https://erp2.example/deploy", events[2].Before.URL)
	require.Nil(t, events[2].After)
}

// A target change whose audit record cannot be written does not stay.
func TestAContractTargetChangeWithoutItsAuditRecordDoesNotStayPostgres(t *testing.T) {
	svc, database, _, ctx := configurationAdminPostgres(t)
	created, err := svc.CreateContractTarget(ctx, &contractworkflowengine.ContractTargetCreateRequest{Name: "ERP", URL: "https://erp.example/deploy"})
	require.NoError(t, err)
	refuseAuditOf(t, database, "UPDATE_CONTRACT_TARGET")

	_, err = svc.UpdateContractTarget(ctx, &contractworkflowengine.ContractTargetUpdateRequest{ID: created.ID, Name: "ERP", URL: "https://elsewhere.example/deploy"})
	require.Error(t, err)

	var url string
	require.NoError(t, database.Get(&url, `SELECT url FROM contract_targets WHERE id = $1`, created.ID))
	require.Equal(t, "https://erp.example/deploy", url)
}

// UC-IF-22: a machine identity change and its audit record commit together. A
// change whose audit record fails does not stay, and a created identity whose
// record fails leaves no OAuth2 client behind.
func TestAMachineIdentityChangeWithoutItsAuditRecordDoesNotStayPostgres(t *testing.T) {
	svc, database, admin, ctx := configurationAdminPostgres(t)
	created, err := svc.CreateMachineIdentity(ctx, &contractworkflowengine.MachineIdentityCreateRequest{
		Name: "ERP", ParticipantDid: thisDeployment, Roles: []string{"Sys. Contract Creator"},
	})
	require.NoError(t, err)
	id := created.Identity.ID

	refuseAuditOf(t, database, "MACHINE_IDENTITY_UPDATED")
	_, err = svc.UpdateMachineIdentity(ctx, &contractworkflowengine.MachineIdentityUpdateRequest{
		ID: id, Name: "ERP", ParticipantDid: thisDeployment, Roles: []string{"Sys. Contract Creator"}, Enabled: false,
	})
	require.Error(t, err)
	var enabled bool
	require.NoError(t, database.Get(&enabled, `SELECT enabled FROM machine_identities WHERE id = $1`, id))
	require.True(t, enabled, "the update was not audited, so it must not stay")

	_, err = database.Exec(`ALTER TABLE outbox_events DROP CONSTRAINT refuse_audit`)
	require.NoError(t, err)
	refuseAuditOf(t, database, "MACHINE_IDENTITY_DELETED")
	require.Error(t, svc.DeleteMachineIdentity(ctx, &contractworkflowengine.MachineIdentityDeleteRequest{ID: id}))
	var remaining int
	require.NoError(t, database.Get(&remaining, `SELECT count(*) FROM machine_identities WHERE id = $1`, id))
	require.Equal(t, 1, remaining)
	require.Empty(t, admin.deleted, "the OAuth2 client stays while the deletion is not recorded")

	_, err = database.Exec(`ALTER TABLE outbox_events DROP CONSTRAINT refuse_audit`)
	require.NoError(t, err)
	refuseAuditOf(t, database, "MACHINE_IDENTITY_SECRET_ROTATED")
	_, err = svc.RotateMachineIdentitySecret(ctx, &contractworkflowengine.MachineIdentityRotateRequest{ID: id})
	require.Error(t, err)
	require.Empty(t, admin.rotated, "the secret stays while the rotation is not recorded")

	_, err = database.Exec(`ALTER TABLE outbox_events DROP CONSTRAINT refuse_audit`)
	require.NoError(t, err)
	refuseAuditOf(t, database, "MACHINE_IDENTITY_CREATED")
	_, err = svc.CreateMachineIdentity(ctx, &contractworkflowengine.MachineIdentityCreateRequest{
		Name: "CRM", ParticipantDid: thisDeployment, Roles: []string{"Sys. Contract Creator"},
	})
	require.Error(t, err)
	var identities int
	require.NoError(t, database.Get(&identities, `SELECT count(*) FROM machine_identities WHERE name = 'CRM'`))
	require.Zero(t, identities)
	require.Equal(t, admin.created[1:], admin.deleted, "the client of the unrecorded identity is removed")
}
