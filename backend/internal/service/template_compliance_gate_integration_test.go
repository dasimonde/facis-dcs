package service

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/templatestructure"
	cwecommand "digital-contracting-service/internal/contractworkflowengine/command"
	"digital-contracting-service/internal/semantichub"
)

// DCS-FR-TR-07: contract creation uses only a template that passed the
// policy audit at approval. A registered template that is NON_COMPLIANT or
// was never assessed (UNASSESSED) is refused.
func TestContractCreationRefusesNonCompliantTemplatePostgres(t *testing.T) {
	f := newContractsFromTemplates(t)

	for _, status := range []string{"NON_COMPLIANT", "UNASSESSED"} {
		did := "tpl-" + status
		createStructuredTemplate(t, f.creator, did, compliantTemplateData, templatestructure.Agreement)
		_, err := f.database.Exec(`UPDATE contract_templates SET state = 'REGISTERED', compliance_status = $2 WHERE did = $1`, did, status)
		require.NoError(t, err)
		require.ErrorIs(t, f.createContract("c-"+status, did), cwecommand.ErrTemplateNotCompliant)
	}

	createStructuredTemplate(t, f.creator, "tpl-bound", compliantTemplateData, templatestructure.Agreement)
	require.Equal(t, "COMPLIANT", f.approveAndRegister("tpl-bound"))
	require.NoError(t, f.createContract("c-bound", "tpl-bound"))
}

// DCS-FR-TR-07: the template policy audit knows the domain fields of every
// active hub ontology, not only the seeded one.
func TestDomainOntologyIncludesRegisteredOntologiesPostgres(t *testing.T) {
	f := newContractsFromTemplates(t)
	hosting, err := os.ReadFile("../../../features/fixtures/facis-sla-hosting-ontology.ttl")
	require.NoError(t, err)
	tx := f.database.MustBegin()
	_, err = (semantichub.Repo{}).Register(f.ctx, tx, "facis-sla-hosting", "ontology", "text/turtle", string(hosting), "org", 0, true)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	content, _, err := (semantichub.HubShapeSource{DB: f.database}).ActiveDomainOntology(f.ctx)
	require.NoError(t, err)
	require.Contains(t, content, "field-service-description", "the seeded SLA ontology")
	require.Contains(t, content, "field-service-serviceCreditRate", "the registered hosting ontology")
}
