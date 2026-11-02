package webhookplatform

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	cweeventtype "digital-contracting-service/internal/contractworkflowengine/datatype/eventtype"
	smeventtype "digital-contracting-service/internal/signingmanagement/datatype/eventtype"
)

// DCS-FR-CSA-23: every contract event a contract owner follows is a webhook
// event an ORCE flow can subscribe to and forward.
func TestOwnerEventsAreSubscribable(t *testing.T) {
	for _, dcsType := range []string{
		cweeventtype.ContractRenewalDue.String(), cweeventtype.ContractExpired.String(), cweeventtype.Terminate.String(),
		cweeventtype.Create.String(), cweeventtype.Submit.String(), cweeventtype.Approve.String(), cweeventtype.Reject.String(),
		cweeventtype.Negotiation.String(), cweeventtype.Offer.String(), cweeventtype.Withdraw.String(),
		smeventtype.Applied.String(), smeventtype.Revoke.String(), cweeventtype.Activate.String(), cweeventtype.Renew.String(),
	} {
		if _, ok := DCSEventMap[dcsType]; !ok {
			t.Errorf("DCSEventMap has no entry for %q", dcsType)
		}
	}
}

// DCS-FR-CSA-23: a contract event names the contract owner, so the receiving
// flow can route it; a template event names none.
func TestDispatchNamesTheContractOwner(t *testing.T) {
	bodies := make(chan []byte, 2)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies <- body
	}))
	defer receiver.Close()
	secret, err := NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	subs := &fakeSubscriptions{subs: []Subscription{
		{ID: "a", Event: "contract.renewal_due", CallbackURL: receiver.URL, Secret: secret, OwnerDID: orgA},
		{ID: "b", Event: "template.updated", CallbackURL: receiver.URL, Secret: secret, OwnerDID: orgA},
	}}
	owner := func(context.Context, string) json.RawMessage {
		return json.RawMessage(`{"created_by":"` + orgA + `","responsible":{"creator":"` + orgA + `"}}`)
	}
	d := NewDispatcher(subs, newMemDeliveries(), admitAll, owner)
	d.httpClient = receiver.Client()

	for _, event := range []string{"contract.renewal_due", "template.updated"} {
		if err := d.Dispatch(context.Background(), "id-"+event, event, "did:web:org-a.example:x:1", nil); err != nil {
			t.Fatal(err)
		}
		var payload struct {
			Event string `json:"event"`
			Owner *struct {
				CreatedBy string `json:"created_by"`
			} `json:"owner"`
		}
		select {
		case body := <-bodies:
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("no delivery of %s", event)
		}
		if event == "contract.renewal_due" && (payload.Owner == nil || payload.Owner.CreatedBy != orgA) {
			t.Fatalf("contract event owner = %+v, want created_by %s", payload.Owner, orgA)
		}
		if event == "template.updated" && payload.Owner != nil {
			t.Fatalf("template event carries owner %+v", payload.Owner)
		}
	}
}
