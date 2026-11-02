package webhookplatform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"digital-contracting-service/internal/base/pgtest"
)

// DCS-NFR-PER-02: with a NATS queue group, the replica that dispatches an event
// is not always the replica that receives the subscriber callback. Each replica
// has its own PostgresDeliveries over the shared database, and both must see
// the same dispatch. Runs only against a real database (QA_DATABASE_URL).
func TestPostgresDeliveriesAreSharedAcrossReplicas(t *testing.T) {
	db := pgtest.Open(t)

	ctx := context.Background()
	dispatching, receiving := PostgresDeliveries{DB: db}, PostgresDeliveries{DB: db}
	owner := "did:web:" + uuid.NewString()
	pending := PendingCallback{EventID: uuid.NewString(), CorrelationID: uuid.NewString(), Event: "contract.created", DID: "did:x", SentAt: time.Now().UTC()}

	delivery := Delivery{SubscriptionID: uuid.NewString(), CallbackURL: "https://a.example", OwnerDID: owner, Payload: []byte(`{}`)}
	ids, err := dispatching.RecordDispatch(ctx, pending, []Delivery{delivery}, time.Now().Add(time.Minute))
	if err != nil || len(ids) != 1 {
		t.Fatalf("RecordDispatch = %v, %v; want one queued delivery", ids, err)
	}
	id := ids[0]
	// NFR-SF-01: a redelivered event carries the same id and is not dispatched again.
	if again, err := receiving.RecordDispatch(ctx, pending, []Delivery{delivery}, time.Now()); err != nil || again != nil {
		t.Fatalf("second RecordDispatch = %v, %v; want none: the event is dispatched already", again, err)
	}
	if err := dispatching.RecordAttempt(ctx, id, 200, "", time.Time{}); err != nil {
		t.Fatalf("RecordAttempt: %v", err)
	}

	if _, ok, err := receiving.ResolvePending(ctx, pending.EventID, "did:web:other"); err != nil || ok {
		t.Fatalf("ResolvePending by an organization that did not receive the event = %v, %v; want false", ok, err)
	}
	resolved, ok, err := receiving.ResolvePending(ctx, pending.EventID, owner)
	if err != nil || !ok || resolved.CorrelationID != pending.CorrelationID {
		t.Fatalf("ResolvePending on another replica = %+v, %v, %v; want the pending dispatch", resolved, ok, err)
	}
	if _, ok, err := dispatching.ResolvePending(ctx, pending.EventID, owner); err != nil || ok {
		t.Fatalf("second ResolvePending = %v, %v; want false: only the first callback resolves a dispatch", ok, err)
	}
	if _, ok, err := receiving.ResolvePending(ctx, "unknown", owner); err != nil || ok {
		t.Fatalf("ResolvePending(unknown) = %v, %v; want false", ok, err)
	}

	got, err := receiving.ListDeliveries(ctx, owner)
	if err != nil || len(got) != 1 {
		t.Fatalf("ListDeliveries on another replica = %+v, %v; want the one delivery", got, err)
	}
	if !got[0].Acknowledged || got[0].Event != "contract.created" || got[0].StatusCode != 200 || got[0].CorrelationID != pending.CorrelationID {
		t.Fatalf("delivery = %+v, want an acknowledged contract.created delivery with HTTP 200", got[0])
	}
	if other, err := receiving.ListDeliveries(ctx, "did:web:other"); err != nil || len(other) != 0 {
		t.Fatalf("ListDeliveries(other org) = %+v, %v; want none", other, err)
	}
}

// UC-IF-27: a queued delivery is due at its retry time until an attempt ends
// it, and an ended delivery keeps no payload.
func TestPostgresDeliveriesAreDueUntilTheyEnd(t *testing.T) {
	db := pgtest.Open(t)
	ctx := context.Background()
	store := PostgresDeliveries{DB: db}
	pending := PendingCallback{EventID: uuid.NewString(), CorrelationID: uuid.NewString(), Event: "contract.created", DID: "did:x", SentAt: time.Now().UTC()}
	ids, err := store.RecordDispatch(ctx, pending, []Delivery{{SubscriptionID: uuid.NewString(), CallbackURL: "https://a.example", OwnerDID: "did:web:a", Payload: []byte(`{"a":1}`)}}, time.Now().Add(time.Minute))
	if err != nil || len(ids) != 1 {
		t.Fatal(ids, err)
	}
	id := ids[0]
	if due, err := store.DueDeliveries(ctx, time.Now(), 10); err != nil || len(due) != 0 {
		t.Fatalf("a delivery is due before its retry time: %+v, %v", due, err)
	}
	due, err := store.DueDeliveries(ctx, time.Now().Add(2*time.Minute), 10)
	if err != nil || len(due) != 1 || string(due[0].Payload) != `{"a":1}` || due[0].CorrelationID != pending.CorrelationID {
		t.Fatalf("due deliveries = %+v, %v; want the queued delivery with its payload", due, err)
	}

	if err := store.RecordAttempt(ctx, id, 503, "", time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if due, _ := store.DueDeliveries(ctx, time.Now().Add(2*time.Minute), 10); len(due) != 1 || due[0].Attempts != 1 {
		t.Fatalf("a refused delivery is not due again: %+v", due)
	}
	if err := store.RecordAttempt(ctx, id, 200, "", time.Time{}); err != nil {
		t.Fatal(err)
	}
	if due, _ := store.DueDeliveries(ctx, time.Now().Add(24*time.Hour), 10); len(due) != 0 {
		t.Fatalf("an ended delivery is still due: %+v", due)
	}
	var payloads int
	if err := db.Get(&payloads, `SELECT count(*) FROM webhook_deliveries WHERE payload IS NOT NULL`); err != nil || payloads != 0 {
		t.Fatalf("an ended delivery keeps its payload: %d, %v", payloads, err)
	}
}

// DCS-NFR-SF-01, ADR-39: webhook delivery is at least once. A replica that
// recorded a dispatch and stopped before it sent leaves the delivery due; the
// redelivered event is not dispatched again, and RetryDue on any replica sends
// the recorded delivery.
func TestARecordedDeliveryIsSentAfterTheReplicaStopped(t *testing.T) {
	db := pgtest.Open(t)
	ctx := context.Background()
	hits := make(chan string, 4)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits <- r.Header.Get("webhook-id")
	}))
	defer receiver.Close()
	secret, err := NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	subID := uuid.NewString()
	subs := &fakeSubscriptions{subs: []Subscription{{ID: subID, Event: "contract.created", CallbackURL: receiver.URL, Secret: secret, OwnerDID: orgA}}}
	store := PostgresDeliveries{DB: db}
	d := NewDispatcher(subs, store, admitAll, nil)
	d.httpClient = receiver.Client()

	// The stopped replica: the dispatch and its delivery are recorded, nothing is sent.
	pending := PendingCallback{EventID: "event-9", CorrelationID: uuid.NewString(), Event: "contract.created", DID: "did:x", SentAt: time.Now().UTC()}
	if _, err := store.RecordDispatch(ctx, pending, []Delivery{{SubscriptionID: subID, CallbackURL: receiver.URL, OwnerDID: orgA, Payload: []byte(`{"event_id":"event-9"}`)}}, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	// JetStream delivers the event again to another replica.
	if err := d.Dispatch(ctx, "event-9", "contract.created", "did:x", nil); err != nil {
		t.Fatal(err)
	}
	select {
	case id := <-hits:
		t.Fatalf("the redelivered event %s was dispatched a second time", id)
	case <-time.After(300 * time.Millisecond):
	}

	if _, err := db.Exec(`UPDATE webhook_deliveries SET next_attempt_at = now() - interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	d.RetryDue(ctx)
	select {
	case id := <-hits:
		if id != "event-9" {
			t.Fatalf("webhook-id %q, want event-9", id)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the recorded delivery was not sent")
	}
	got, err := store.ListDeliveries(ctx, orgA)
	if err != nil || len(got) != 1 || got[0].StatusCode != http.StatusOK || got[0].Attempts != 1 {
		t.Fatalf("deliveries = %+v, %v; want one delivery accepted on its first attempt", got, err)
	}
}
