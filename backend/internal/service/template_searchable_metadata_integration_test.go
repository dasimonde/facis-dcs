package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/semantichub"
	"digital-contracting-service/internal/templaterepository/command"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatetype"
	pg "digital-contracting-service/internal/templaterepository/db/pg"
	"digital-contracting-service/internal/templaterepository/query/contracttemplate"
)

// DCS-FR-TR-10: templates are searchable by jurisdiction, industry and the
// regulatory compliance status the policy audit sets at approval.
func TestTemplateSearchableMetadataPostgres(t *testing.T) {
	database := pgtest.Open(t)
	ctx := context.Background()
	require.NoError(t, semantichub.Seed(ctx, database))
	validation.SetShapeSource(semantichub.HubShapeSource{DB: database})
	ctRepo := &pg.PostgresContractTemplateRepo{}

	create := func(did, jurisdiction, industry string) {
		t.Helper()
		data := datatype.JSON(compliantTemplateData)
		require.NoError(t, (&command.Creator{DB: database, CTRepo: ctRepo}).Handle(ctx, command.CreateCmd{
			DID: did, CreatedBy: "org", TemplateType: contracttemplatetype.ContractTemplate, PartyType: "SINGLE_PARTY",
			TemplateData: &data, Jurisdiction: &jurisdiction, Industry: &industry,
			UserRoles: userrole.UserRoles{userrole.TemplateCreator},
		}))
	}
	create("tpl-de-energy", "DEU", "Energy")
	create("tpl-fr-cloud", "FRA", "Cloud hosting")

	_, err := database.Exec(`UPDATE contract_templates SET state = 'REVIEWED' WHERE did = 'tpl-de-energy'`)
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO contract_templates_approval_task (did,state,approver,created_by) VALUES ('tpl-de-energy','OPEN','org','org')`)
	require.NoError(t, err)
	require.NoError(t, (&command.Approver{DB: database, CTRepo: ctRepo, ATRepo: &pg.PostgresApprovalTaskRepo{}}).Handle(ctx, command.ApproveCmd{
		DID: "tpl-de-energy", UpdatedAt: time.Now().Add(time.Hour), ApprovedBy: "org", UserRoles: userrole.UserRoles{userrole.TemplateApprover},
	}))

	search := func(qry contracttemplate.GetAllMetadataByFilterQry) []string {
		t.Helper()
		result, err := (&contracttemplate.GetAllMetaDataByFilterHandler{DB: database, CTRepo: ctRepo}).Handle(ctx, qry)
		require.NoError(t, err)
		dids := []string{}
		for _, item := range result {
			dids = append(dids, item.DID)
		}
		return dids
	}
	require.Equal(t, []string{"tpl-de-energy"}, search(contracttemplate.GetAllMetadataByFilterQry{Jurisdiction: "deu"}))
	require.Equal(t, []string{"tpl-fr-cloud"}, search(contracttemplate.GetAllMetadataByFilterQry{Industry: "cloud"}))
	require.Equal(t, []string{"tpl-de-energy"}, search(contracttemplate.GetAllMetadataByFilterQry{ComplianceStatus: "COMPLIANT"}))
	require.Equal(t, []string{"tpl-fr-cloud"}, search(contracttemplate.GetAllMetadataByFilterQry{ComplianceStatus: "UNASSESSED"}))
}

// DCS-IR-TR-05, DCS-IR-TR-06: an error finding of the template policy audit
// refuses the approval. The template stays REVIEWED with an open approval task.
func TestApproveRefusesNonCompliantTemplatePostgres(t *testing.T) {
	database := pgtest.Open(t)
	ctx := context.Background()
	require.NoError(t, semantichub.Seed(ctx, database))
	validation.SetShapeSource(semantichub.HubShapeSource{DB: database})
	ctRepo := &pg.PostgresContractTemplateRepo{}

	// The minimal template has no clause bound to contract data.
	data := datatype.JSON(sodTemplateData)
	require.NoError(t, (&command.Creator{DB: database, CTRepo: ctRepo}).Handle(ctx, command.CreateCmd{
		DID: "tpl", CreatedBy: "org", TemplateType: contracttemplatetype.ContractTemplate, PartyType: "SINGLE_PARTY",
		TemplateData: &data, UserRoles: userrole.UserRoles{userrole.TemplateCreator},
	}))
	_, err := database.Exec(`UPDATE contract_templates SET state = 'REVIEWED' WHERE did = 'tpl'`)
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO contract_templates_approval_task (did,state,approver,created_by) VALUES ('tpl','OPEN','org','org')`)
	require.NoError(t, err)

	err = (&command.Approver{DB: database, CTRepo: ctRepo, ATRepo: &pg.PostgresApprovalTaskRepo{}}).Handle(ctx, command.ApproveCmd{
		DID: "tpl", UpdatedAt: time.Now().Add(time.Hour), ApprovedBy: "org", UserRoles: userrole.UserRoles{userrole.TemplateApprover},
	})
	require.ErrorIs(t, err, command.ErrTemplateNotCompliant)
	require.Contains(t, err.Error(), "FACIS-TPL-CLAUSE-001")
	require.Equal(t, "bad_request", errorName(mapTemplateCommandError(err)))

	var state, task, status string
	require.NoError(t, database.Get(&state, `SELECT state FROM contract_templates WHERE did = 'tpl'`))
	require.NoError(t, database.Get(&status, `SELECT compliance_status FROM contract_templates WHERE did = 'tpl'`))
	require.NoError(t, database.Get(&task, `SELECT state FROM contract_templates_approval_task WHERE did = 'tpl'`))
	require.Equal(t, "REVIEWED", state)
	require.Equal(t, "UNASSESSED", status)
	require.Equal(t, "OPEN", task)
}

// DCS-IR-TR-06: register and publish refuse a template that has not passed
// the template policy audit.
func TestRegisterAndPublishRefuseNonCompliantTemplatePostgres(t *testing.T) {
	database := pgtest.Open(t)
	ctx := context.Background()
	ctRepo := &pg.PostgresContractTemplateRepo{}
	for _, row := range [][2]string{{"approved", "APPROVED"}, {"registered", "REGISTERED"}} {
		_, err := database.Exec(`INSERT INTO contract_templates (did,created_by,state,template_type,party_type,name,template_data,compliance_status) VALUES ($1,'creator',$2,'CONTRACT_TEMPLATE','SINGLE_PARTY','n','{}','NON_COMPLIANT')`, row[0], row[1])
		require.NoError(t, err)
	}

	_, err := (&command.Registrar{DB: database, CTRepo: ctRepo, VCSigner: echoVCSigner{}, IssuerDID: "did:web:issuer.example"}).
		Handle(ctx, command.RegisterCmd{DID: "approved", RegisteredBy: "manager"})
	require.ErrorIs(t, err, command.ErrTemplateNotCompliant)

	err = (&command.Publisher{DB: database, CTRepo: ctRepo}).Handle(ctx, command.PublishCmd{
		DID: "registered", UpdatedAt: time.Now().Add(time.Hour), PublishedBy: "manager", InstanceDID: "did:web:issuer.example",
	})
	require.ErrorIs(t, err, command.ErrTemplateNotCompliant)

	var states []string
	require.NoError(t, database.Select(&states, `SELECT state FROM contract_templates ORDER BY did`))
	require.Equal(t, []string{"APPROVED", "REGISTERED"}, states)
}

func TestTemplateComplianceStatus(t *testing.T) {
	require.Equal(t, "COMPLIANT", command.ComplianceStatus([]validation.PolicyFinding{{Severity: validation.SeverityWarning}}))
	require.Equal(t, "NON_COMPLIANT", command.ComplianceStatus([]validation.PolicyFinding{{Severity: validation.SeverityWarning}, {Severity: validation.SeverityError}}))
}
