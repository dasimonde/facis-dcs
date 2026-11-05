package service

import (
	"context"
	"encoding/json"
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
	"digital-contracting-service/internal/templaterepository/db"
	pg "digital-contracting-service/internal/templaterepository/db/pg"
	"digital-contracting-service/internal/templaterepository/query/contracttemplate"
)

func TestTemplateSearchUsesSemanticValuesPostgres(t *testing.T) {
	database := pgtest.Open(t)
	ctx := context.Background()
	require.NoError(t, semantichub.Seed(ctx, database))
	defer validation.SwapShapeSource(semantichub.HubShapeSource{DB: database})()
	repo := &pg.PostgresContractTemplateRepo{}
	for did, jurisdiction := range map[string]string{"tpl-de": "DEU", "tpl-fr": "FRA"} {
		var doc map[string]any
		require.NoError(t, json.Unmarshal([]byte(compliantTemplateData), &doc))
		fields, _ := doc["dcs:contractFields"].([]any)
		doc["dcs:contractFields"] = append(fields, map[string]any{"@id": "urn:uuid:00000000-0000-4000-8000-000000000001", "@type": "dcs:ContractField", "dcs:label": "Jurisdiction", "dcs:datatype": "xsd:string", "dcs:required": false, "dcs:shape": map[string]any{"@id": "https://w3id.org/facis/dcs/taxonomy/v1#field-contract-jurisdiction"}, "dcs:value": jurisdiction})
		raw, err := json.Marshal(doc)
		require.NoError(t, err)
		data := datatype.JSON(raw)
		require.NoError(t, (&command.Creator{DB: database, CTRepo: repo}).Handle(ctx, command.CreateCmd{DID: did, CreatedBy: "org", TemplateType: contracttemplatetype.ContractTemplate, PartyType: "SINGLE_PARTY", TemplateData: &data, UserRoles: userrole.UserRoles{userrole.TemplateCreator}}))
	}
	rows, err := (&contracttemplate.GetAllMetaDataByFilterHandler{DB: database, CTRepo: repo}).Handle(ctx, contracttemplate.GetAllMetadataByFilterQry{SemanticFilters: `[{"filter_id":"jurisdiction","value":"DEU"}]`})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "tpl-de", rows[0].DID)
	tx, err := database.BeginTxx(ctx, nil)
	require.NoError(t, err)
	_, err = repo.CopyFromDID(ctx, tx, "tpl-de", "tpl-copy")
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	rows, err = (&contracttemplate.GetAllMetaDataByFilterHandler{DB: database, CTRepo: repo}).Handle(ctx, contracttemplate.GetAllMetadataByFilterQry{SemanticFilters: `[{"filter_id":"jurisdiction","value":"DEU"}]`})
	require.NoError(t, err)
	require.Len(t, rows, 2)
	changed := datatype.JSON(`{"dcs:contractFields":[{"dcs:shape":{"@id":"https://w3id.org/facis/dcs/taxonomy/v1#field-contract-jurisdiction"},"dcs:value":"ESP"}]}`)
	tx, err = database.BeginTxx(ctx, nil)
	require.NoError(t, err)
	require.NoError(t, repo.Update(ctx, tx, db.ContractTemplateUpdateData{DID: "tpl-copy", TemplateData: &changed}))
	require.NoError(t, tx.Commit())
	rows, err = (&contracttemplate.GetAllMetaDataByFilterHandler{DB: database, CTRepo: repo}).Handle(ctx, contracttemplate.GetAllMetadataByFilterQry{SemanticFilters: `[{"filter_id":"jurisdiction","value":"DEU"}]`})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "tpl-de", rows[0].DID)
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
