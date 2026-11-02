package webhookplatform

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	cloudevent "github.com/cloudevents/sdk-go/v2/event"
)

func TestDeliveryIsSignedOverTheSentBody(t *testing.T) {
	var got struct {
		header http.Header
		body   []byte
	}
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.header = r.Header.Clone()
		got.body, _ = io.ReadAll(r.Body)
	}))
	defer receiver.Close()

	secret, err := NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	subs := &fakeSubscriptions{subs: []Subscription{{ID: "s", Event: "template.updated", CallbackURL: receiver.URL, Secret: secret, OwnerDID: orgA}}}
	store := newMemDeliveries()
	d := NewDispatcher(subs, store, admitAll, nil)
	d.httpClient = receiver.Client()
	if err := d.Dispatch(context.Background(), "evt-signed", "template.updated", "did:web:org-a.example:template:1", json.RawMessage(`{"name":"a<b&c","version":1.0}`)); err != nil {
		t.Fatal(err)
	}
	delivered := waitForAttempts(t, store, orgA, 1)
	payload := delivered[0]

	if auth := got.header.Get("Authorization"); auth != "" {
		t.Fatalf("delivery carries Authorization %q", auth)
	}
	if id := got.header.Get("webhook-id"); id != payload.EventID {
		t.Fatalf("webhook-id = %q, want %q", id, payload.EventID)
	}
	timestamp := got.header.Get("webhook-timestamp")
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || time.Since(time.Unix(ts, 0)).Abs() > time.Minute {
		t.Fatalf("webhook-timestamp = %q, want the current unix time in seconds", timestamp)
	}
	want, err := Signature(secret, payload.EventID, timestamp, got.body)
	if err != nil || got.header.Get("webhook-signature") != want {
		t.Fatalf("webhook-signature = %q, want %q over the received body", got.header.Get("webhook-signature"), want)
	}
}

// Contract events carry the contract content: only organizations the audience
// admits receive them.
func TestDispatchDeliversOnlyToAdmittedOrganizations(t *testing.T) {
	hits := make(chan string, 2)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits <- r.URL.Path
	}))
	defer receiver.Close()

	secret, err := NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	subs := &fakeSubscriptions{subs: []Subscription{
		{ID: "a", Event: "contract.created", CallbackURL: receiver.URL + "/a", Secret: secret, OwnerDID: orgA},
		{ID: "b", Event: "contract.created", CallbackURL: receiver.URL + "/b", Secret: secret, OwnerDID: orgB},
	}}
	audience := func(_ context.Context, event, did, owner string) bool {
		return event == "contract.created" && did == "contract-1" && owner == orgA
	}
	d := NewDispatcher(subs, newMemDeliveries(), audience, nil)
	d.httpClient = receiver.Client()

	if err := d.Dispatch(context.Background(), "event-1", "contract.created", "contract-1", json.RawMessage(`{"contract_data":{}}`)); err != nil {
		t.Fatal(err)
	}

	select {
	case path := <-hits:
		if path != "/a" {
			t.Fatalf("delivered to %s, want only /a", path)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no delivery to the admitted organization")
	}
	select {
	case path := <-hits:
		t.Fatalf("second delivery to %s: org B is not admitted", path)
	case <-time.After(300 * time.Millisecond):
	}
}

// DCS-NFR-SF-01: JetStream delivers an event at least once. A redelivered
// event has the same CloudEvent id, which is the webhook event id, and is not
// sent to the subscriber a second time.
func TestRedeliveredEventIsDispatchedOnce(t *testing.T) {
	hits := make(chan string, 2)
	receiver := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		hits <- r.Header.Get("webhook-id")
	}))
	defer receiver.Close()
	secret, err := NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	subs := &fakeSubscriptions{subs: []Subscription{
		{ID: "a", Event: "contract.created", CallbackURL: receiver.URL, Secret: secret, OwnerDID: orgA},
	}}
	d := NewDispatcher(subs, newMemDeliveries(), admitAll, nil)
	d.httpClient = receiver.Client()

	evt := cloudevent.New()
	evt.SetID("event-7")
	evt.SetSource("CONTRACT_WORKFLOW_ENGINE")
	evt.SetType("CREATE_CONTRACT")
	if err := evt.SetData(cloudevent.ApplicationJSON, json.RawMessage(`{"did":"contract-1"}`)); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := handleNATSEvent(context.Background(), evt, d); err != nil {
			t.Fatal(err)
		}
	}

	select {
	case id := <-hits:
		if id != "event-7" {
			t.Fatalf("webhook-id %q, want the CloudEvent id", id)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no delivery")
	}
	select {
	case <-hits:
		t.Fatal("the redelivered event was sent again")
	case <-time.After(300 * time.Millisecond):
	}
}

func TestDefaultClientRefusesLoopbackCallbacks(t *testing.T) {
	receiver := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("the dispatcher reached a loopback callback")
	}))
	defer receiver.Close()
	secret, err := NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	store := newMemDeliveries()
	subs := &fakeSubscriptions{subs: []Subscription{{ID: "s", Event: "template.updated", CallbackURL: receiver.URL, Secret: secret, OwnerDID: orgA}}}
	d := NewDispatcher(subs, store, admitAll, nil)
	if err := d.Dispatch(context.Background(), "evt-loopback", "template.updated", "did:web:org-a.example:template:1", nil); err != nil {
		t.Fatal(err)
	}
	got := waitForAttempts(t, store, orgA, 1)
	// UC-IF-25: the subscriber reads that the URL was not reached, not the
	// dial error, which names addresses inside the deployment.
	if got[0].Error != errUnreachable || got[0].StatusCode != 0 {
		t.Fatalf("deliveries = %+v, want one refused delivery reported as %q", got, errUnreachable)
	}
}

// UC-IF-27: a delivery is recorded before its first attempt and sent again
// until the subscriber accepts it: after a refused attempt, and when the
// dispatching replica stopped before its first attempt.
func TestADeliveryIsRetriedUntilTheSubscriberAcceptsIt(t *testing.T) {
	answers := make(chan int, 3)
	answers <- http.StatusServiceUnavailable
	answers <- http.StatusOK
	answers <- http.StatusOK
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(<-answers)
	}))
	defer receiver.Close()
	secret, err := NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	subs := &fakeSubscriptions{subs: []Subscription{{ID: "s", Event: "template.updated", CallbackURL: receiver.URL, Secret: secret, OwnerDID: orgA}}}
	store := newMemDeliveries()
	d := NewDispatcher(subs, store, admitAll, nil)
	d.httpClient = receiver.Client()
	ctx := context.Background()

	if err := d.Dispatch(ctx, "evt-retried", "template.updated", "did:web:org-a.example:template:1", nil); err != nil {
		t.Fatal(err)
	}
	refused := waitForAttempts(t, store, orgA, 1)
	if refused[0].StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("first attempt = %+v, want the 503", refused[0])
	}
	later := time.Now().Add(2 * time.Hour)
	if due, _ := store.DueDeliveries(ctx, later, 10); len(due) != 1 {
		t.Fatalf("the refused delivery is not due again: %+v", due)
	}

	// A delivery queued by a replica that stopped before its first attempt.
	stopped := store.queue(Delivery{EventID: "lost", SubscriptionID: "s", CallbackURL: receiver.URL, OwnerDID: orgA, Payload: []byte(`{}`)}, time.Now())

	store.mu.Lock()
	for id := range store.due {
		store.due[id] = time.Now().Add(-time.Second)
	}
	store.mu.Unlock()
	d.RetryDue(ctx)

	got, _ := store.ListDeliveries(ctx, orgA)
	for _, delivery := range got {
		if delivery.StatusCode != http.StatusOK {
			t.Fatalf("delivery %+v was not sent again until accepted", delivery)
		}
	}
	if got[stopped-1].Attempts != 1 || got[0].Attempts != 2 {
		t.Fatalf("attempts = %d and %d, want 2 for the refused and 1 for the queued delivery", got[0].Attempts, got[stopped-1].Attempts)
	}
	if due, _ := store.DueDeliveries(ctx, later, 10); len(due) != 0 {
		t.Fatalf("accepted deliveries are still due: %+v", due)
	}
}

// waitForAttempts waits until ownerDID's deliveries have at least n attempts
// in total and returns them.
func waitForAttempts(t *testing.T, store *memDeliveries, ownerDID string, n int) []Delivery {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		got, _ := store.ListDeliveries(context.Background(), ownerDID)
		total := 0
		for _, d := range got {
			total += d.Attempts
		}
		if total >= n {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("deliveries = %+v, want %d attempts", got, n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
