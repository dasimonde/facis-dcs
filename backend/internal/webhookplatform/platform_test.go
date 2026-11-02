package webhookplatform

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

const (
	orgA = "did:web:org-a.example"
	orgB = "did:web:org-b.example"
)

type fakeSubscriptions struct {
	mu   sync.Mutex
	subs []Subscription
}

func (f *fakeSubscriptions) Add(_ context.Context, sub Subscription) (Subscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sub.ID = uuid.New().String()
	sub.CreatedAt = time.Now().UTC()
	f.subs = append(f.subs, sub)
	return sub, nil
}

func (f *fakeSubscriptions) ListByOwner(_ context.Context, ownerDID string) ([]Subscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Subscription
	for _, s := range f.subs {
		if s.OwnerDID == ownerDID {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeSubscriptions) DeleteOwned(_ context.Context, id, ownerDID string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, s := range f.subs {
		if s.ID == id && s.OwnerDID == ownerDID {
			f.subs = append(f.subs[:i], f.subs[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeSubscriptions) ByEvent(_ context.Context, event string) ([]Subscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []Subscription
	for _, s := range f.subs {
		if s.Event == event {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeSubscriptions) ByID(_ context.Context, id string) (*Subscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.subs {
		if s.ID == id {
			return &s, nil
		}
	}
	return nil, nil
}

func admitAll(context.Context, string, string, string) bool { return true }

// Each test token is the name of a caller in this table.
var testCallers = map[string]Caller{
	"manager-a": {HolderDID: "user-a", ParticipantDID: orgA, Roles: []string{"Integration Manager"}},
	"admin-b":   {HolderDID: "user-b", ParticipantDID: orgB, Roles: []string{"Sys. Administrator"}},
	"template-a": {HolderDID: "user-t", ParticipantDID: orgA,
		Roles: []string{"Template Manager", "Contract Manager"}},
	"no-org": {HolderDID: "user-x", Roles: []string{"Integration Manager"}},
	// A machine identity: HolderDID is its client_id (middleware/oidc.go).
	"orce-a":        {HolderDID: "dcs-orce-integration", ParticipantDID: orgA, Roles: []string{"Sys. Integration Manager"}},
	"orce-notary-a": {HolderDID: "dcs-orce-notary", ParticipantDID: orgA, Roles: []string{"Sys. Auditor"}},
}

func newTestPlatform(subs Subscriptions) *Platform {
	store := newMemDeliveries()
	validate := func(_ context.Context, token string) (Caller, error) {
		c, ok := testCallers[token]
		if !ok {
			return Caller{}, errors.New("unknown token")
		}
		return c, nil
	}
	return New(subs, store, NewDispatcher(subs, store, admitAll, nil), validate, nil)
}

func do(t *testing.T, p *Platform, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)
	return rec
}

func subscribe(t *testing.T, p *Platform, token string) (id, secret string) {
	t.Helper()
	rec := do(t, p, http.MethodPost, "/webhooks", token,
		`{"event":"template.updated","callback_url":"https://receiver.example/hook"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /webhooks as %s = %d %s, want 201", token, rec.Code, rec.Body)
	}
	var sub struct{ ID, Secret string }
	if err := json.Unmarshal(rec.Body.Bytes(), &sub); err != nil || sub.ID == "" {
		t.Fatalf("POST /webhooks response has no id: %s", rec.Body)
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(sub.Secret, "whsec_"))
	if !strings.HasPrefix(sub.Secret, "whsec_") || err != nil || len(key) != 32 {
		t.Fatalf("POST /webhooks secret = %q, want whsec_ and a base64 32-byte key", sub.Secret)
	}
	return sub.ID, sub.Secret
}

func list(t *testing.T, p *Platform, token string) []map[string]any {
	t.Helper()
	rec := do(t, p, http.MethodGet, "/webhooks", token, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /webhooks as %s = %d %s, want 200", token, rec.Code, rec.Body)
	}
	var subs []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &subs); err != nil {
		t.Fatalf("decode GET /webhooks: %v", err)
	}
	return subs
}

func TestOwnerSubscribesListsAndDeletes(t *testing.T) {
	subs := &fakeSubscriptions{}
	p := newTestPlatform(subs)

	id, secret := subscribe(t, p, "manager-a")
	if got := subs.subs[0]; got.OwnerDID != orgA || got.CreatedBy != "user-a" || got.Secret != secret {
		t.Fatalf("stored subscription = %+v, want owner %s, creator user-a and the returned secret", got, orgA)
	}
	if _, second := subscribe(t, p, "manager-a"); second == secret {
		t.Fatalf("two subscriptions got the same secret")
	}

	listed := list(t, p, "manager-a")
	if len(listed) != 2 || listed[0]["id"] != id {
		t.Fatalf("GET /webhooks = %v, want the two subscriptions, %s first", listed, id)
	}
	if _, ok := listed[0]["secret"]; ok {
		t.Fatalf("GET /webhooks returns the secret: %v", listed[0])
	}

	if rec := do(t, p, http.MethodDelete, "/webhooks/"+id, "manager-a", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE own subscription = %d %s, want 204", rec.Code, rec.Body)
	}
	if len(subs.subs) != 1 {
		t.Fatalf("subscription is still stored after DELETE: %+v", subs.subs)
	}
}

// DCS-NFR-SQ-08: an integration manages the subscriptions of its organization
// with a machine credential of the role Sys. Integration Manager. Another
// machine role does not.
func TestMachineIntegrationManagesItsSubscriptions(t *testing.T) {
	subs := &fakeSubscriptions{}
	p := newTestPlatform(subs)

	id, _ := subscribe(t, p, "orce-a")
	if got := subs.subs[0]; got.OwnerDID != orgA || got.CreatedBy != "dcs-orce-integration" {
		t.Fatalf("stored subscription = %+v, want owner %s, creator dcs-orce-integration", got, orgA)
	}
	if listed := list(t, p, "manager-a"); len(listed) != 1 || listed[0]["id"] != id {
		t.Fatalf("the organization's Integration Manager does not see the machine's subscription: %v", listed)
	}
	if rec := do(t, p, http.MethodGet, "/deliveries", "orce-a", ""); rec.Code != http.StatusOK {
		t.Fatalf("GET /deliveries as the machine integration = %d, want 200", rec.Code)
	}
	if rec := do(t, p, http.MethodPost, "/webhooks", "orce-notary-a",
		`{"event":"template.updated","callback_url":"https://x.example/hook"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("POST /webhooks as Sys. Auditor = %d, want 403", rec.Code)
	}
	if rec := do(t, p, http.MethodDelete, "/webhooks/"+id, "orce-a", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE as the machine integration = %d, want 204", rec.Code)
	}
}

// DCS generates the secret. A request that brings its own is refused, so the
// client does not verify with a key DCS never signs with.
func TestSubscribeWithOwnSecretIsRejected(t *testing.T) {
	subs := &fakeSubscriptions{}
	p := newTestPlatform(subs)
	for _, secret := range []string{`"0123456789abcdef0123456789abcdef"`, `""`, `null`} {
		rec := do(t, p, http.MethodPost, "/webhooks", "manager-a",
			`{"event":"template.updated","callback_url":"https://receiver.example/hook","secret":`+secret+`}`)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("POST /webhooks with secret %s = %d, want 400", secret, rec.Code)
		}
	}
	if len(subs.subs) != 0 {
		t.Fatalf("a request with its own secret was stored: %+v", subs.subs)
	}
}

func TestForeignOrganizationCannotSeeOrDeleteSubscription(t *testing.T) {
	subs := &fakeSubscriptions{}
	p := newTestPlatform(subs)
	id, _ := subscribe(t, p, "manager-a")

	if listed := list(t, p, "admin-b"); len(listed) != 0 {
		t.Fatalf("GET /webhooks for org B shows org A's subscriptions: %v", listed)
	}
	if rec := do(t, p, http.MethodDelete, "/webhooks/"+id, "admin-b", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("DELETE of org A's subscription by org B = %d, want 404", rec.Code)
	}
	if len(subs.subs) != 1 {
		t.Fatalf("org B removed org A's subscription")
	}
}

func TestCallerWithoutManagementRoleIsForbidden(t *testing.T) {
	subs := &fakeSubscriptions{}
	p := newTestPlatform(subs)
	id, _ := subscribe(t, p, "manager-a")

	for _, token := range []string{"template-a", "no-org"} {
		for _, tc := range []struct{ method, path, body string }{
			{http.MethodGet, "/webhooks", ""},
			{http.MethodPost, "/webhooks", `{"event":"template.updated","callback_url":"https://x.example"}`},
			{http.MethodDelete, "/webhooks/" + id, ""},
			{http.MethodGet, "/deliveries", ""},
		} {
			if rec := do(t, p, tc.method, tc.path, token, tc.body); rec.Code != http.StatusForbidden {
				t.Errorf("%s %s as %s = %d, want 403", tc.method, tc.path, token, rec.Code)
			}
		}
	}
	if len(subs.subs) != 1 {
		t.Fatalf("a caller without a management role changed the subscriptions: %+v", subs.subs)
	}
}

func TestSubscribeRejectsCallbackURLsThatAreNotHTTPS(t *testing.T) {
	subs := &fakeSubscriptions{}
	p := newTestPlatform(subs)
	// UC-IF-25: a delivery carries contract content, so plain http is refused
	// for a host the deployment does not name.
	for _, u := range []string{"ftp://receiver.example/hook", "/relative", "https://user:pw@receiver.example/hook", "http://", "http://receiver.example/hook"} {
		rec := do(t, p, http.MethodPost, "/webhooks", "manager-a",
			`{"event":"template.updated","callback_url":"`+u+`"}`)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("POST /webhooks with callback_url %q = %d, want 400", u, rec.Code)
		}
	}
	if len(subs.subs) != 0 {
		t.Fatalf("an invalid callback URL was stored: %+v", subs.subs)
	}
}

func TestDeliveriesListOnlyTheCallerOrganization(t *testing.T) {
	store := newMemDeliveries()
	store.queue(Delivery{EventID: "a", CallbackURL: "https://a.example", OwnerDID: orgA}, time.Now())
	store.queue(Delivery{EventID: "b", CallbackURL: "https://b.example", OwnerDID: orgB}, time.Now())
	validate := func(_ context.Context, token string) (Caller, error) { return testCallers[token], nil }
	p := New(&fakeSubscriptions{}, store, NewDispatcher(&fakeSubscriptions{}, store, admitAll, nil), validate, nil)

	rec := do(t, p, http.MethodGet, "/deliveries", "manager-a", "")
	var got []map[string]any
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil {
		t.Fatalf("GET /deliveries = %d %s, want 200", rec.Code, rec.Body)
	}
	if len(got) != 1 || got[0]["event_id"] != "a" {
		t.Fatalf("GET /deliveries for org A = %v, want only org A's delivery", got)
	}
}

// UC-IF-26: a callback needs a management role, and it acknowledges only the
// deliveries of the caller's organization. Another organization's callback
// for the same event_id finds nothing to acknowledge.
func TestCallbackAcknowledgesOnlyTheCallerOrganizationDeliveries(t *testing.T) {
	store := newMemDeliveries()
	ctx := context.Background()
	if _, err := store.RecordDispatch(ctx, PendingCallback{EventID: "evt", Event: "contract.created"},
		[]Delivery{{EventID: "evt", CallbackURL: "https://a.example", OwnerDID: orgA}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	validate := func(_ context.Context, token string) (Caller, error) { return testCallers[token], nil }
	p := New(&fakeSubscriptions{}, store, NewDispatcher(&fakeSubscriptions{}, store, admitAll, nil), validate, nil)
	body := `{"event_id":"evt","status":"done"}`

	if rec := do(t, p, http.MethodPost, "/callbacks", "template-a", body); rec.Code != http.StatusForbidden {
		t.Fatalf("POST /callbacks without a management role = %d, want 403", rec.Code)
	}
	if rec := do(t, p, http.MethodPost, "/callbacks", "admin-b", body); rec.Code != http.StatusNotFound {
		t.Fatalf("POST /callbacks by an organization that did not receive the event = %d, want 404", rec.Code)
	}
	if got, _ := store.ListDeliveries(ctx, orgA); got[0].Acknowledged {
		t.Fatal("another organization acknowledged org A's delivery")
	}
	if rec := do(t, p, http.MethodPost, "/callbacks", "manager-a", body); rec.Code != http.StatusOK {
		t.Fatalf("POST /callbacks by the receiving organization = %d %s, want 200", rec.Code, rec.Body)
	}
	if got, _ := store.ListDeliveries(ctx, orgA); !got[0].Acknowledged {
		t.Fatal("the receiving organization's callback did not acknowledge its delivery")
	}
}

// A host the deployment names may receive deliveries over plain http.
func TestSubscribeAcceptsPlainHTTPForANamedHost(t *testing.T) {
	t.Setenv("DCS_WEBHOOK_INSECURE_HOSTS", "receiver.example:8080")
	p := newTestPlatform(&fakeSubscriptions{})
	rec := do(t, p, http.MethodPost, "/webhooks", "manager-a",
		`{"event":"template.updated","callback_url":"http://receiver.example:8080/hook"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /webhooks with a named plain-http host = %d %s, want 201", rec.Code, rec.Body)
	}
}
