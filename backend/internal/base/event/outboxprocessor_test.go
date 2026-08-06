package event

import (
	"context"
	"testing"
	"time"

	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/datatype"
)

// shreddedKeys is a CEK repository whose every scope has already been
// destroyed, which is what Encrypt meets for a contract erased while one of its
// events was still queued.
type shreddedKeys struct{}

func (shreddedKeys) Fetch(_ context.Context, scope artifactstore.Scope, recipientDID string) (*artifactstore.CEKRecord, error) {
	shreddedAt := time.Now().UTC()
	return &artifactstore.CEKRecord{
		ScopeKind: string(scope.Kind), ScopeID: scope.ID, RecipientDID: recipientDID,
		ShreddedAt: &shreddedAt,
	}, nil
}

func (shreddedKeys) List(_ context.Context, _ artifactstore.Scope) ([]artifactstore.CEKRecord, error) {
	return nil, nil
}

func (shreddedKeys) Insert(_ context.Context, _ artifactstore.Scope, _ string, _ []byte) (bool, error) {
	return false, nil
}

func (shreddedKeys) Shred(_ context.Context, _ artifactstore.Scope, _, _ string) (int64, error) {
	return 0, nil
}

// Erasure destroys a contract's content. It must not also destroy the record
// that something happened to that contract: an event still queued when the CEK
// was shredded could never be encrypted, so it was retried to its dead-letter
// budget and dropped out of the tamper-evident trail entirely.
func TestEventOfAnErasedContractIsAnchoredWithAnErasedBody(t *testing.T) {
	store := artifactstore.New(nil, shreddedKeys{}, nil, "did:web:me", "did:web:me#dcs-ecdh", nil)
	j := OutboxProcessor{Artifacts: store}
	contractIRI := "did:web:me:contract:1"

	body, scopeKind, scopeID, err := j.sealBody(context.Background(), datatype.OutboxEvent{
		ID: 87, Component: "CONTRACT_STORAGE_ARCHIVE", EventType: "PAC_AUDIT_EXECUTED",
		DID: &contractIRI, EventData: []byte(`{"contract":"secret"}`),
	})

	if err != nil {
		t.Fatalf("an event of an erased contract must still anchor, got %v", err)
	}
	if string(body) != string(datatype.ErasedEventData) {
		t.Errorf("body = %s, want the defined erased marker %s", body, datatype.ErasedEventData)
	}
	// No key exists, so naming a scope would promise a body a reader could open.
	if scopeKind != "" || scopeID != "" {
		t.Errorf("erased entry names CEK scope %q/%q, want none", scopeKind, scopeID)
	}
}

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
