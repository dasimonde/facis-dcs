package dcstodcs

import (
	"encoding/json"
	"testing"

	cloudevent "github.com/cloudevents/sdk-go/v2/event"
)

func submitEvent(t *testing.T, body string) cloudevent.Event {
	t.Helper()
	evt := cloudevent.New()
	evt.SetID("1")
	evt.SetSource("CONTRACT_WORKFLOW_ENGINE")
	evt.SetType("SUBMIT_CONTRACT")
	if err := evt.SetData(cloudevent.ApplicationJSON, json.RawMessage(body)); err != nil {
		t.Fatal(err)
	}
	return evt
}

func TestSettlementFromEventReadsTheNegotiatedSubmission(t *testing.T) {
	did, settled, err := settlementFromEvent(submitEvent(t,
		`{"did":"did:web:dcs-a.localhost:contract:42","previous_state":"NEGOTIATION","new_state":"SUBMITTED"}`))
	if err != nil {
		t.Fatalf("expected the submit event to be readable, got: %v", err)
	}
	if !settled {
		t.Fatal("NEGOTIATION -> SUBMITTED is the settlement, it must ship one")
	}
	if did != "did:web:dcs-a.localhost:contract:42" {
		t.Fatalf("settlement read for the wrong contract: %s", did)
	}
}

// SUBMITTED is also reached from REVIEWED, when an approver sends a contract
// back for another review round. Nothing was settled there, and shipping a
// settlement for it would tell the counterparty this instance agreed a
// document it is still working through.
func TestSettlementFromEventIgnoresASubmissionThatSettledNothing(t *testing.T) {
	for _, body := range []string{
		`{"did":"did:web:dcs-a.localhost:contract:42","previous_state":"REVIEWED","new_state":"SUBMITTED"}`,
		`{"did":"did:web:dcs-a.localhost:contract:42","previous_state":"SUBMITTED","new_state":"SUBMITTED"}`,
		`{"did":"did:web:dcs-a.localhost:contract:42","previous_state":"NEGOTIATION","new_state":"NEGOTIATION"}`,
	} {
		_, settled, err := settlementFromEvent(submitEvent(t, body))
		if err != nil {
			t.Fatalf("expected %s to be readable, got: %v", body, err)
		}
		if settled {
			t.Fatalf("%s is not a settlement", body)
		}
	}
}

func TestSettlementFromEventRefusesAnUnreadableEvent(t *testing.T) {
	if _, _, err := settlementFromEvent(submitEvent(t, `{"previous_state":"NEGOTIATION","new_state":"SUBMITTED"}`)); err == nil {
		t.Fatal("an event naming no contract must be an error, not a silently skipped ship")
	}
	if _, _, err := settlementFromEvent(submitEvent(t, `"not an object"`)); err == nil {
		t.Fatal("an undecodable event must be an error")
	}
}
