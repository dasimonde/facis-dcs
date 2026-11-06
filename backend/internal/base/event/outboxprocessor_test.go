package event

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/datatype"
)

// DCS-NFR-SF-01: an event that cannot be published stops the batch, so no
// later event goes out ahead of it; the next run starts with it again.
func TestPublishingStopsAtTheFirstFailedEvent(t *testing.T) {
	events := []datatype.OutboxEvent{{ID: 1, EventType: "a"}, {ID: 2, EventType: "b"}, {ID: 3, EventType: "c"}}
	var sent []string
	var marked []int64
	err := publishInOrder(events, func(_, _, eventType string, _ json.RawMessage) error {
		if eventType == "b" {
			return errors.New("no connection")
		}
		sent = append(sent, eventType)
		return nil
	}, func(id int64) error {
		marked = append(marked, id)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sent) != 1 || sent[0] != "a" || len(marked) != 1 || marked[0] != 1 {
		t.Fatalf("sent %v, marked %v; want only the event before the failed one", sent, marked)
	}
}

// DCS-NFR-SF-01: a batch whose commit fails is published again. The event
// keeps its CloudEvent id, so the stream stores it once and the consumers
// recognize it.
func TestRepublishedEventKeepsItsCloudEventID(t *testing.T) {
	created := time.Date(2026, 10, 31, 12, 0, 0, 123456000, time.UTC)
	event := datatype.OutboxEvent{ID: 7, CreatedAt: created}
	var ids []string
	publish := func(id, _, _ string, _ json.RawMessage) error {
		ids = append(ids, id)
		return nil
	}
	mark := func(int64) error { return nil }
	for range 2 {
		if err := publishInOrder([]datatype.OutboxEvent{event}, publish, mark); err != nil {
			t.Fatal(err)
		}
	}
	other := cloudEventID(datatype.OutboxEvent{ID: 8, CreatedAt: created})
	if ids[0] == "" || ids[0] != ids[1] || ids[0] == other {
		t.Fatalf("ids %v (other event %s); want one stable id per event", ids, other)
	}
}

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
