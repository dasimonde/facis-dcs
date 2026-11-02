package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/base/templatestructure"
	"digital-contracting-service/internal/base/validation"
	cwecommand "digital-contracting-service/internal/contractworkflowengine/command"
	cwepg "digital-contracting-service/internal/contractworkflowengine/db/pg"
	syncpg "digital-contracting-service/internal/dcstodcs/db/pg"
	"digital-contracting-service/internal/semantichub"
	"digital-contracting-service/internal/templaterepository/command"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatetype"
	"digital-contracting-service/internal/templaterepository/db"
	pg "digital-contracting-service/internal/templaterepository/db/pg"
)

// compliantTemplateData has a clause bound to a declared contract field, so
// the template policy audit finds no error. It is the BDD fixture template.
const compliantTemplateData = `{
  "@context": {"dcs": "https://w3id.org/facis/dcs/ontology/v1#"},
  "@type": "dcs:ContractTemplate",
  "dcs:metadata": {"@type": "dcs:TemplateMetadata", "dcs:title": "Compliant"},
  "dcs:documentStructure": {
    "@type": "dcs:DocumentStructure",
    "dcs:blocks": {"@list": [{"@id": "urn:uuid:block-clause-1", "@type": "dcs:Clause",
      "dcs:content": {"@list": ["Confidentiality clause", {"@id": "urn:uuid:field-service-description"}]}}]},
    "dcs:layout": [{"@id": "urn:uuid:block-root", "@type": "dcs:LayoutNode", "dcs:isRoot": true,
      "dcs:children": {"@list": [{"@id": "urn:uuid:block-clause-1"}]}}]
  },
  "dcs:contractFields": [{"@id": "urn:uuid:field-service-description", "@type": "dcs:ContractField",
    "dcs:label": "Service description", "dcs:datatype": "xsd:string", "dcs:required": true,
    "dcs:shape": {"@id": "https://w3id.org/facis/dcs/taxonomy/v1#field-service-description"},
    "dcs:value": "BDD fixture service"}],
  "dcs:policies": []
}`

func createStructuredTemplate(t *testing.T, creator *command.Creator, did, data string, level templatestructure.Level, requires ...string) {
	t.Helper()
	raw := datatype.JSON(data)
	require.NoError(t, creator.Handle(context.Background(), command.CreateCmd{
		DID: did, CreatedBy: "org", TemplateType: contracttemplatetype.ContractTemplate, PartyType: "SINGLE_PARTY",
		TemplateData: &raw, TemplateLevel: string(level), RequiredTemplates: requires,
		UserRoles: userrole.UserRoles{userrole.TemplateCreator},
	}))
}

// DCS-FR-TR-02, DCS-FR-TR-23, DCS-FR-TR-26: a template has a level and the
// templates it requires; submission and approval check them.
func TestTemplateLevelsAndDependenciesPostgres(t *testing.T) {
	database := pgtest.Open(t)
	ctx := context.Background()
	ctRepo, rtRepo, atRepo := &pg.PostgresContractTemplateRepo{}, &pg.PostgresReviewTaskRepo{}, &pg.PostgresApprovalTaskRepo{}
	creator := &command.Creator{DB: database, CTRepo: ctRepo}
	submitter := &command.Submitter{DB: database, CTRepo: ctRepo, RTRepo: rtRepo, ATRepo: atRepo}
	future := time.Now().Add(time.Hour)
	submit := func(did string) error {
		return submitter.Handle(ctx, command.SubmitCmd{DID: did, UpdatedAt: future, SubmittedBy: "org", UserRoles: userrole.UserRoles{userrole.TemplateCreator}})
	}
	setState := func(did, state string) {
		t.Helper()
		_, err := database.Exec(`UPDATE contract_templates SET state = $2 WHERE did = $1`, did, state)
		require.NoError(t, err)
	}

	createStructuredTemplate(t, creator, "frame", sodTemplateData, templatestructure.FrameAgreement)
	createStructuredTemplate(t, creator, "sub", sodTemplateData, templatestructure.SubAgreement, "frame")
	createStructuredTemplate(t, creator, "plain", sodTemplateData, "")
	createStructuredTemplate(t, creator, "upward", sodTemplateData, templatestructure.FrameAgreement, "sub")
	createStructuredTemplate(t, creator, "ghost", sodTemplateData, templatestructure.Addendum, "does-not-exist")

	read := func(did string) *db.ContractTemplate {
		t.Helper()
		tx := database.MustBegin()
		defer func() { require.NoError(t, tx.Rollback()) }()
		stored, err := ctRepo.ReadDataByID(ctx, tx, did)
		require.NoError(t, err)
		return stored
	}
	require.Equal(t, "SUB_AGREEMENT", read("sub").TemplateLevel)
	require.Equal(t, []string{"frame"}, []string(read("sub").RequiredTemplates))
	require.Equal(t, "AGREEMENT", read("plain").TemplateLevel)

	require.ErrorIs(t, submit("sub"), templatestructure.ErrInvalid, "the required frame has no approved version")
	require.ErrorIs(t, submit("ghost"), templatestructure.ErrInvalid)
	setState("frame", "REGISTERED")
	setState("sub", "DRAFT")
	require.NoError(t, submit("sub"))
	setState("sub", "REGISTERED")
	require.ErrorIs(t, submit("upward"), templatestructure.ErrInvalid, "a frame agreement depended on a sub-agreement")

	// Approval checks again: the frame lost its usable version after submission.
	createStructuredTemplate(t, creator, "addendum", sodTemplateData, templatestructure.Addendum, "frame")
	require.NoError(t, submit("addendum"))
	setState("addendum", "REVIEWED")
	setState("frame", "DEPRECATED")
	err := (&command.Approver{DB: database, CTRepo: ctRepo, ATRepo: atRepo}).Handle(ctx, command.ApproveCmd{
		DID: "addendum", UpdatedAt: future, ApprovedBy: "org", UserRoles: userrole.UserRoles{userrole.TemplateApprover},
	})
	require.ErrorIs(t, err, templatestructure.ErrInvalid)

	// A new version keeps the level and the dependencies.
	tx := database.MustBegin()
	_, err = ctRepo.CopyFromDID(ctx, tx, "sub", "sub-v2")
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	require.Equal(t, "SUB_AGREEMENT", read("sub-v2").TemplateLevel)
	require.Equal(t, []string{"frame"}, []string(read("sub-v2").RequiredTemplates))
}

// contractsFromTemplates approves and registers templates and creates
// contracts from them on a seeded hub.
type contractsFromTemplates struct {
	t        *testing.T
	ctx      context.Context
	database *sqlx.DB
	creator  *command.Creator
	document *identity.DIDDocument
}

func newContractsFromTemplates(t *testing.T) *contractsFromTemplates {
	database := pgtest.Open(t)
	ctx := context.Background()
	require.NoError(t, semantichub.Seed(ctx, database))
	validation.SetShapeSource(semantichub.HubShapeSource{DB: database})
	return &contractsFromTemplates{
		t: t, ctx: ctx, database: database,
		creator:  &command.Creator{DB: database, CTRepo: &pg.PostgresContractTemplateRepo{}},
		document: jadesTestDIDDocument(t, "local"),
	}
}

// approveAndRegister returns the compliance status the approval set.
func (f *contractsFromTemplates) approveAndRegister(did string) string {
	f.t.Helper()
	_, err := f.database.Exec(`UPDATE contract_templates SET state = 'REVIEWED' WHERE did = $1`, did)
	require.NoError(f.t, err)
	_, err = f.database.Exec(`INSERT INTO contract_templates_approval_task (did,state,approver,created_by) VALUES ($1,'OPEN','org','org')`, did)
	require.NoError(f.t, err)
	require.NoError(f.t, (&command.Approver{DB: f.database, CTRepo: &pg.PostgresContractTemplateRepo{}, ATRepo: &pg.PostgresApprovalTaskRepo{}}).Handle(f.ctx, command.ApproveCmd{
		DID: did, UpdatedAt: time.Now().Add(time.Hour), ApprovedBy: "org", UserRoles: userrole.UserRoles{userrole.TemplateApprover},
	}))
	var status string
	require.NoError(f.t, f.database.Get(&status, `SELECT compliance_status FROM contract_templates WHERE did = $1`, did))
	_, err = f.database.Exec(`UPDATE contract_templates SET state = 'REGISTERED' WHERE did = $1`, did)
	require.NoError(f.t, err)
	return status
}

func (f *contractsFromTemplates) createContract(did, templateDID string) error {
	return (&cwecommand.Creator{
		DB: f.database, CRepo: &cwepg.PostgresContractRepo{}, CTRepo: &cwepg.PostgresContractTemplateRepo{},
		RTRepo: &cwepg.PostgresReviewTaskRepo{}, ATRepo: &cwepg.PostgresApprovalTaskRepo{}, NTRepo: &cwepg.PostgresNegotiationTaskRepo{},
		DIDDocument: *f.document,
	}).Handle(f.ctx, cwecommand.CreateCmd{DID: did, TemplateDID: templateDID, CreatedBy: "Org A", Counterparty: "did:web:peer"})
}

// setParent saves the stored contract document with a dcs:parentContract.
func (f *contractsFromTemplates) setParent(did, parent string) error {
	f.t.Helper()
	var stored struct {
		UpdatedAt time.Time     `db:"updated_at"`
		Data      datatype.JSON `db:"contract_data"`
	}
	require.NoError(f.t, f.database.Get(&stored, `SELECT updated_at, contract_data FROM contracts WHERE did = $1`, did))
	var doc map[string]any
	require.NoError(f.t, json.Unmarshal(stored.Data, &doc))
	doc["dcs:parentContract"] = map[string]any{"@id": parent}
	data, err := datatype.NewJSON(doc)
	require.NoError(f.t, err)
	peer, err := f.document.GetID()
	require.NoError(f.t, err)
	return (&cwecommand.Updater{DB: f.database, CRepo: &cwepg.PostgresContractRepo{}, SRepo: syncpg.PostgresSyncRepository{}, DIDDocument: *f.document}).
		Handle(f.ctx, cwecommand.UpdateCmd{DID: did, UpdatedAt: stored.UpdatedAt.Add(time.Second), UpdatedBy: "Org A", ContractData: &data, CauserDID: peer})
}

// DCS-FR-TR-23, DCS-FR-TR-26: a contract parent fits the template levels and
// the required templates, and a contract is created only while the templates
// its template requires are usable.
func TestContractParentFollowsTemplateLevelsPostgres(t *testing.T) {
	f := newContractsFromTemplates(t)
	createStructuredTemplate(t, f.creator, "tpl-frame", compliantTemplateData, templatestructure.FrameAgreement)
	createStructuredTemplate(t, f.creator, "tpl-plain", compliantTemplateData, templatestructure.Agreement)
	createStructuredTemplate(t, f.creator, "tpl-sub", compliantTemplateData, templatestructure.SubAgreement, "tpl-frame")
	for _, pair := range [][2]string{{"c-frame", "tpl-frame"}, {"c-plain", "tpl-plain"}, {"c-sub", "tpl-sub"}} {
		require.Equal(t, "COMPLIANT", f.approveAndRegister(pair[1]))
		require.NoError(t, f.createContract(pair[0], pair[1]))
	}

	require.ErrorIs(t, f.setParent("c-sub", "c-plain"), templatestructure.ErrInvalid, "a sub-agreement got an agreement as parent")
	require.NoError(t, f.setParent("c-sub", "c-frame"))

	_, err := f.database.Exec(`UPDATE contract_templates SET state = 'DEPRECATED' WHERE did = 'tpl-frame'`)
	require.NoError(t, err)
	require.ErrorIs(t, f.createContract("c-sub-2", "tpl-sub"), templatestructure.ErrInvalid)
}
