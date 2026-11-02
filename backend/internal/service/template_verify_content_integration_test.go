package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	templaterepository "digital-contracting-service/gen/template_repository"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/middleware"
	"digital-contracting-service/internal/semantichub"
	pg "digital-contracting-service/internal/templaterepository/db/pg"
	"digital-contracting-service/internal/templaterepository/query"
)

// DCS-IR-TR-03, DCS-FR-TR-20, DCS-FR-TR-21: verify runs the template policy
// audit of the approval and the Semantic Hub shapes. Error findings keep the
// review task open, warnings do not, a broken envelope is a finding, and
// every verification is logged with its findings.
func TestTemplateVerifyRunsPolicyAndShapesPostgres(t *testing.T) {
	database := pgtest.Open(t)
	ctx := context.Background()
	require.NoError(t, semantichub.Seed(ctx, database))
	validation.SetShapeSource(semantichub.HubShapeSource{DB: database})
	verifier := &query.Verifier{DB: database, CTRepo: &pg.PostgresContractTemplateRepo{}, RTRepo: &pg.PostgresReviewTaskRepo{}}
	const reviewer = "did:web:org.example"

	seed := func(did, data string) {
		t.Helper()
		_, err := database.Exec(`INSERT INTO contract_templates (did,created_by,state,template_type,party_type,name,template_data) VALUES ($1,'creator','SUBMITTED','CONTRACT_TEMPLATE','SINGLE_PARTY','n',$2)`, did, data)
		require.NoError(t, err)
		_, err = database.Exec(`INSERT INTO contract_templates_review_task (did,state,reviewer,created_by) VALUES ($1,'OPEN',$2,$2)`, did, reviewer)
		require.NoError(t, err)
	}
	verify := func(did string) *query.VerifyResult {
		t.Helper()
		result, err := verifier.Handle(ctx, query.VerifyQry{DID: did, VerifiedBy: reviewer, UserRoles: userrole.UserRoles{userrole.TemplateReviewer}})
		require.NoError(t, err)
		return result
	}
	taskState := func(did string) string {
		t.Helper()
		var state string
		require.NoError(t, database.Get(&state, `SELECT state FROM contract_templates_review_task WHERE did = $1`, did))
		return state
	}
	requireFinding := func(findings []string, part string) {
		t.Helper()
		for _, finding := range findings {
			if strings.Contains(finding, part) {
				return
			}
		}
		t.Fatalf("no finding containing %q in %v", part, findings)
	}

	// The minimal template has no clause bound to contract data.
	seed("policy", sodTemplateData)
	result := verify("policy")
	requireFinding(result.Findings, "FACIS-TPL-CLAUSE-001")
	require.Equal(t, "OPEN", taskState("policy"))

	// Two inline values break the hub's dcs:ContractFieldShape.
	seed("shapes", strings.Replace(compliantTemplateData, `"dcs:value": "BDD fixture service"`, `"dcs:value": ["a", "b"]`, 1))
	result = verify("shapes")
	requireFinding(result.Findings, "at most one inline dcs:value")
	require.Equal(t, "OPEN", taskState("shapes"))

	seed("envelope", `{"@context": {"dcs": "https://w3id.org/facis/dcs/ontology/v1#"}, "@type": "dcs:ContractTemplate"}`)
	result = verify("envelope")
	requireFinding(result.Findings, "template data is not valid")
	require.Equal(t, "OPEN", taskState("envelope"))

	seed("compliant", compliantTemplateData)
	result = verify("compliant")
	require.Empty(t, result.Findings)
	requireFinding(result.Warnings, "FACIS-TPL-LEGAL-001")
	require.Equal(t, "VERIFIED", taskState("compliant"))

	var logged []string
	require.NoError(t, database.Select(&logged, `SELECT event_data::text FROM outbox_events WHERE event_type = 'VERIFY_CONTRACT_TEMPLATE' AND did = 'policy'`))
	require.Len(t, logged, 1)
	var evt struct {
		Findings []string `json:"findings"`
	}
	require.NoError(t, json.Unmarshal([]byte(logged[0]), &evt))
	requireFinding(evt.Findings, "FACIS-TPL-CLAUSE-001")
}

// DCS-FR-TR-21: a read of the provenance credentials is logged.
func TestTemplateProvenanceReadIsLoggedPostgres(t *testing.T) {
	database := pgtest.Open(t)
	_, err := database.Exec(`INSERT INTO contract_templates (did,created_by,state,template_type,party_type,name) VALUES ('tpl','creator','REGISTERED','CONTRACT_TEMPLATE','SINGLE_PARTY','n')`)
	require.NoError(t, err)
	svc := &templateRepositorysrvc{DB: database, CTRepo: &pg.PostgresContractTemplateRepo{}}
	ctx := middleware.InjectAuthContext(context.Background(), []string{userrole.ContractCreator.String()}, "did:key:holder", "did:web:org.example")

	_, err = svc.Provenance(ctx, &templaterepository.TemplateProvenanceRetrieveRequest{Did: "tpl"})
	require.NoError(t, err)

	var holder string
	require.NoError(t, database.Get(&holder, `SELECT event_data->>'holder_did' FROM outbox_events WHERE event_type = 'RETRIEVE_CONTRACT_TEMPLATE_PROVENANCE' AND did = 'tpl'`))
	require.Equal(t, "did:key:holder", holder)
}
