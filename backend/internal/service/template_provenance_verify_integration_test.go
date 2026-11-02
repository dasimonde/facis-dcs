package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/pgtest"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/semantichub"
	"digital-contracting-service/internal/templaterepository/command"
	pg "digital-contracting-service/internal/templaterepository/db/pg"
	"digital-contracting-service/internal/templaterepository/query"
)

// DCS-FR-TR-09, DCS-FR-TR-20: verify checks the provenance credentials and
// that the content still matches the hash sealed at registration; the
// credential of a later version links the one of the version before.
func TestTemplateVerifyAgainstProvenanceCredentialPostgres(t *testing.T) {
	database := pgtest.Open(t)
	ctx := context.Background()
	require.NoError(t, semantichub.Seed(ctx, database))
	validation.SetShapeSource(semantichub.HubShapeSource{DB: database})
	ctRepo := &pg.PostgresContractTemplateRepo{}
	registrar := &command.Registrar{
		DB: database, CTRepo: ctRepo,
		VCSigner: echoVCSigner{}, IssuerDID: "did:web:issuer.example",
	}
	var proofErr error
	verifier := &query.Verifier{DB: database, CTRepo: ctRepo, RTRepo: &pg.PostgresReviewTaskRepo{},
		VerifyCredential: func(json.RawMessage) error { return proofErr }}
	creator := userrole.UserRoles{userrole.ContractCreator}

	insert := func(did, base string, version int, state string) {
		t.Helper()
		_, err := database.Exec(`INSERT INTO contract_templates (did,created_by,state,template_type,party_type,name,template_data,version,base_template,compliance_status) VALUES ($1,'creator',$2,'CONTRACT_TEMPLATE','SINGLE_PARTY','n',$3,$4,$5,'COMPLIANT')`, did, state, compliantTemplateData, version, base)
		require.NoError(t, err)
	}
	findings := func(did string) []string {
		t.Helper()
		result, err := verifier.Handle(ctx, query.VerifyQry{DID: did, VerifiedBy: "org", UserRoles: creator})
		require.NoError(t, err)
		return result.Findings
	}
	requireFinding := func(got []string, part string) {
		t.Helper()
		for _, finding := range got {
			if strings.Contains(finding, part) {
				return
			}
		}
		t.Fatalf("no finding containing %q in %v", part, got)
	}

	insert("tpl-v1", "tpl-v1", 1, "APPROVED")
	_, err := registrar.Handle(ctx, command.RegisterCmd{DID: "tpl-v1", RegisteredBy: "manager"})
	require.NoError(t, err)
	require.Empty(t, findings("tpl-v1"))

	proofErr = errors.New("proof mismatch")
	requireFinding(findings("tpl-v1"), "does not verify")
	proofErr = nil

	_, err = database.Exec(`UPDATE contract_templates SET template_data = jsonb_set(template_data, '{dcs:metadata,dcs:title}', '"Altered"') WHERE did = 'tpl-v1'`)
	require.NoError(t, err)
	requireFinding(findings("tpl-v1"), "changed since approval")

	insert("tpl-unsealed", "tpl-unsealed", 1, "REGISTERED")
	requireFinding(findings("tpl-unsealed"), "no provenance credential")

	insert("tpl-v2", "tpl-v1", 2, "APPROVED")
	_, err = registrar.Handle(ctx, command.RegisterCmd{DID: "tpl-v2", RegisteredBy: "manager"})
	require.NoError(t, err)
	var v1, previous string
	require.NoError(t, database.Get(&v1, `SELECT vc_id FROM template_provenance_credentials WHERE did='tpl-v1'`))
	require.NoError(t, database.Get(&previous, `SELECT previous_vc_id FROM template_provenance_credentials WHERE did='tpl-v2'`))
	require.Equal(t, v1, previous)
}
