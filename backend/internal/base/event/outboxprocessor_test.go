package event

import (
	"testing"

	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/datatype"
)

func TestScopeForEvent(t *testing.T) {
	store := artifactstore.New(nil, nil, nil, "did:web:me", "did:web:me#dcs-ecdh", nil)
	j := OutboxProcessor{Artifacts: store}

	contractIRI := "did:web:me:contract:1"
	templateIRI := "did:web:me:template:1"
	star := "*"

	cases := []struct {
		name  string
		event datatype.OutboxEvent
		want  artifactstore.Scope
	}{
		{"contract component", datatype.OutboxEvent{Component: "CONTRACT_WORKFLOW_ENGINE", DID: &contractIRI}, artifactstore.ContractScope(contractIRI)},
		{"signature component", datatype.OutboxEvent{Component: "SIGNATURE_MANAGEMENT", DID: &contractIRI}, artifactstore.ContractScope(contractIRI)},
		{"archive component", datatype.OutboxEvent{Component: "CONTRACT_STORAGE_ARCHIVE", DID: &contractIRI}, artifactstore.ContractScope(contractIRI)},
		{"template component", datatype.OutboxEvent{Component: "CONTRACT_TEMPLATE_REPOSITORY", DID: &templateIRI}, artifactstore.TemplateScope(templateIRI)},
		{"catalogue component", datatype.OutboxEvent{Component: "TEMPLATE_CATALOGUE_INTEGRATION", DID: &templateIRI}, artifactstore.TemplateScope(templateIRI)},
		{"no resource DID", datatype.OutboxEvent{Component: "CONTRACT_WORKFLOW_ENGINE", DID: &star}, store.InstanceScope()},
		{"nil DID", datatype.OutboxEvent{Component: "SIGNATURE_MANAGEMENT"}, store.InstanceScope()},
		{"system component", datatype.OutboxEvent{Component: "SYSTEM", DID: &contractIRI}, store.InstanceScope()},
		{"pac component", datatype.OutboxEvent{Component: "PROCESS_AUDIT_AND_COMPLIANCE", DID: &contractIRI}, store.InstanceScope()},
	}
	for _, tc := range cases {
		if got := j.scopeForEvent(tc.event); got != tc.want {
			t.Errorf("%s: scope = %+v, want %+v", tc.name, got, tc.want)
		}
	}
}
