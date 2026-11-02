package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/datatype/userrole"
	cwedb "digital-contracting-service/internal/contractworkflowengine/db"
	"digital-contracting-service/internal/templaterepository/command"
	pg "digital-contracting-service/internal/templaterepository/db/pg"
)

// DCS-FR-TR-17: when version 2 of a template is registered, new contracts are
// made from version 2 only. DCS-FR-TR-22: the registration emits
// UPDATE_CONTRACT_TEMPLATE ("template.updated") for version 1 with the
// successor's DID.
func TestTemplateSuccessorReplacesPredecessorPostgres(t *testing.T) {
	f := newContractsFromTemplates(t)
	createStructuredTemplate(t, f.creator, "tpl-v1", compliantTemplateData, "")
	require.Equal(t, "COMPLIANT", f.approveAndRegister("tpl-v1"))
	require.NoError(t, f.createContract("c-before", "tpl-v1"))

	ctRepo := &pg.PostgresContractTemplateRepo{}
	tx := f.database.MustBegin()
	version, err := ctRepo.CopyFromDID(f.ctx, tx, "tpl-v1", "tpl-v2")
	require.NoError(t, err)
	require.Equal(t, 2, version)
	require.NoError(t, tx.Commit())
	_, err = f.database.Exec(`UPDATE contract_templates SET state = 'REVIEWED' WHERE did = 'tpl-v2'`)
	require.NoError(t, err)
	_, err = f.database.Exec(`INSERT INTO contract_templates_approval_task (did,state,approver,created_by) VALUES ('tpl-v2','OPEN','org','org')`)
	require.NoError(t, err)
	require.NoError(t, (&command.Approver{DB: f.database, CTRepo: ctRepo, ATRepo: &pg.PostgresApprovalTaskRepo{}}).Handle(f.ctx, command.ApproveCmd{
		DID: "tpl-v2", UpdatedAt: time.Now().Add(time.Hour), ApprovedBy: "org", UserRoles: userrole.UserRoles{userrole.TemplateApprover},
	}))
	_, err = (&command.Registrar{DB: f.database, CTRepo: ctRepo, VCSigner: echoVCSigner{}, IssuerDID: "did:web:issuer.example"}).
		Handle(f.ctx, command.RegisterCmd{DID: "tpl-v2", RegisteredBy: "org"})
	require.NoError(t, err)

	require.ErrorIs(t, f.createContract("c-outdated", "tpl-v1"), cwedb.ErrTemplateNotUsable)
	require.NoError(t, f.createContract("c-current", "tpl-v2"))

	var successor string
	require.NoError(t, f.database.Get(&successor, `SELECT event_data->>'successor_did' FROM outbox_events WHERE event_type = 'UPDATE_CONTRACT_TEMPLATE' AND did = 'tpl-v1'`))
	require.Equal(t, "tpl-v2", successor)
}
