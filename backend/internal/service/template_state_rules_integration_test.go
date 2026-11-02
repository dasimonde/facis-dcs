package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/templaterepository/command"
	pg "digital-contracting-service/internal/templaterepository/db/pg"
)

type echoVCSigner struct{}

func (echoVCSigner) CreateCredential(_ context.Context, unsigned json.RawMessage) (json.RawMessage, error) {
	return unsigned, nil
}

// DCS-IR-TR-06, DCS-FR-TR-14, DCS-FR-TR-17: only an approved template is registered.
func TestTemplateRegisterRequiresApprovedPostgres(t *testing.T) {
	database := pgtest.Open(t)
	ctx := context.Background()
	registrar := &command.Registrar{
		DB: database, CTRepo: &pg.PostgresContractTemplateRepo{},
		VCSigner: echoVCSigner{}, IssuerDID: "did:web:issuer.example",
	}
	cases := map[string]string{
		"DRAFT": "", "SUBMITTED": "", "REJECTED": "", "REVIEWED": "", "PUBLISHED": "", "DEPRECATED": "", "DELETED": "",
		"APPROVED": "REGISTERED", "REGISTERED": "REGISTERED",
	}
	for state, want := range cases {
		t.Run(state, func(t *testing.T) {
			did := "register-" + state
			_, err := database.Exec(`INSERT INTO contract_templates (did,created_by,state,template_type,party_type,name,template_data,compliance_status) VALUES ($1,'creator',$2,'CONTRACT_TEMPLATE','SINGLE_PARTY','n','{}','COMPLIANT')`, did, state)
			require.NoError(t, err)
			_, err = registrar.Handle(ctx, command.RegisterCmd{DID: did, RegisteredBy: "manager", UserRoles: userrole.UserRoles{userrole.TemplateManager}})
			var stored string
			require.NoError(t, database.Get(&stored, `SELECT state FROM contract_templates WHERE did=$1`, did))
			if want == "" {
				require.ErrorIs(t, err, command.ErrInvalidTemplateState)
				require.Equal(t, state, stored)
				return
			}
			require.NoError(t, err)
			require.Equal(t, want, stored)
		})
	}
}

// DCS-FR-TR-17, DCS-FR-TR-18, DCS-IR-TR-07: archive deprecates a live
// template and deletes a template that never entered the pool or a
// deprecated one. A template in review or approved is refused. Deprecation
// and deletion are separate events with the states and the justification,
// and a deprecated version keeps its review and approval records.
func TestTemplateArchiveDeletesDeprecatedPostgres(t *testing.T) {
	database := pgtest.Open(t)
	ctx := context.Background()
	archiver := &command.Archiver{DB: database, CTRepo: &pg.PostgresContractTemplateRepo{}, RTRepo: &pg.PostgresReviewTaskRepo{}, ATRepo: &pg.PostgresApprovalTaskRepo{}}
	future := time.Now().Add(time.Hour)
	cases := map[string]string{
		"REGISTERED": "DEPRECATED", "PUBLISHED": "DEPRECATED",
		"DRAFT": "DELETED", "REJECTED": "DELETED", "DEPRECATED": "DELETED",
		"SUBMITTED": "", "REVIEWED": "", "APPROVED": "", "DELETED": "",
	}
	for state, want := range cases {
		t.Run(state, func(t *testing.T) {
			did := "archive-" + state
			_, err := database.Exec(`INSERT INTO contract_templates (did,created_by,state,template_type,party_type,name) VALUES ($1,'creator',$2,'CONTRACT_TEMPLATE','SINGLE_PARTY','n')`, did, state)
			require.NoError(t, err)
			_, err = database.Exec(`INSERT INTO contract_templates_review_task (did,state,reviewer,created_by) VALUES ($1,'APPROVED','org','org')`, did)
			require.NoError(t, err)
			err = archiver.Handle(ctx, command.ArchiveCmd{DID: did, UpdatedAt: future, ArchivedBy: "manager", Justification: "outdated", UserRoles: userrole.UserRoles{userrole.TemplateManager}})
			var stored string
			require.NoError(t, database.Get(&stored, `SELECT state FROM contract_templates WHERE did=$1`, did))
			if want == "" {
				require.ErrorIs(t, err, command.ErrInvalidTemplateState)
				require.Equal(t, state, stored)
				return
			}
			require.NoError(t, err)
			require.Equal(t, want, stored)

			var evt struct {
				Type          string `db:"event_type"`
				PreviousState string `db:"previous_state"`
				NewState      string `db:"new_state"`
				Justification string `db:"justification"`
			}
			require.NoError(t, database.Get(&evt, `SELECT event_type, event_data->>'previous_state' AS previous_state, event_data->>'new_state' AS new_state, event_data->>'justification' AS justification FROM outbox_events WHERE did = $1`, did))
			wantType := map[string]string{"DEPRECATED": "ARCHIVE_CONTRACT_TEMPLATE", "DELETED": "DELETE_CONTRACT_TEMPLATE"}[want]
			require.Equal(t, wantType, evt.Type)
			require.Equal(t, state, evt.PreviousState)
			require.Equal(t, want, evt.NewState)
			require.Equal(t, "outdated", evt.Justification)

			var tasks int
			require.NoError(t, database.Get(&tasks, `SELECT count(*) FROM contract_templates_review_task WHERE did = $1`, did))
			require.Equal(t, map[string]int{"DEPRECATED": 1, "DELETED": 0}[want], tasks)
		})
	}

	require.ErrorIs(t, archiver.Handle(ctx, command.ArchiveCmd{DID: "archive-REGISTERED", UpdatedAt: future, ArchivedBy: "manager", Justification: " "}), command.ErrInvalidRequest)
}

// DCS-IR-TR-07: a deprecated template that open contracts were made from is
// not deleted.
func TestTemplateDeletionRefusedWhileInUsePostgres(t *testing.T) {
	f := newContractsFromTemplates(t)
	createStructuredTemplate(t, f.creator, "tpl", compliantTemplateData, "")
	require.Equal(t, "COMPLIANT", f.approveAndRegister("tpl"))
	require.NoError(t, f.createContract("contract", "tpl"))
	_, err := f.database.Exec(`UPDATE contract_templates SET state = 'DEPRECATED' WHERE did = 'tpl'`)
	require.NoError(t, err)

	archiver := &command.Archiver{DB: f.database, CTRepo: &pg.PostgresContractTemplateRepo{}, RTRepo: &pg.PostgresReviewTaskRepo{}, ATRepo: &pg.PostgresApprovalTaskRepo{}}
	archive := command.ArchiveCmd{DID: "tpl", UpdatedAt: time.Now().Add(time.Hour), ArchivedBy: "manager", Justification: "outdated"}
	require.ErrorIs(t, archiver.Handle(f.ctx, archive), command.ErrTemplateInUse)

	_, err = f.database.Exec(`UPDATE contracts SET state = 'TERMINATED' WHERE did = 'contract'`)
	require.NoError(t, err)
	require.NoError(t, archiver.Handle(f.ctx, archive))
}
