// Package changefeed passes contract changes from the event bus to the live
// dashboard streams of this replica (DCS-FR-CWE-24, DCS-FR-CSA-23).
package changefeed

import (
	"encoding/json"
	"sync"

	cloudevent "github.com/cloudevents/sdk-go/v2/event"

	"digital-contracting-service/internal/base/event"
)

// Change names a contract and the audit event that changed it.
type Change struct {
	DID       string
	EventType string
}

// changeEvents are the audit events that change what a dashboard shows. Read
// events are left out: a dashboard that refetches after a change writes one,
// and streaming it would start the refetch again. PEER_EXCHANGE is the event
// of a change that a counterparty instance sent (an offer, a counteroffer, a
// signed copy, a settlement).
var changeEvents = map[string]bool{
	"CREATE_CONTRACT": true, "UPDATE_CONTRACT": true, "SUBMIT_CONTRACT": true,
	"OFFER_CONTRACT": true, "WITHDRAW_CONTRACT": true, "NEGOTIATE_CONTRACT": true,
	"ACCEPT_OFFER_CONTRACT": true, "ACCEPT_RESPOND_CONTRACT": true, "REJECT_RESPOND_CONTRACT": true,
	"WITHDRAW_RESPOND_CONTRACT": true, "INCREASE_CONTRACT_VERSION": true, "APPROVE_CONTRACT": true,
	"REJECT_CONTRACT": true, "TERMINATE_CONTRACT": true, "RENEW_CONTRACT": true,
	"CONTRACT_EXPIRED": true, "CONTRACT_RENEWAL_DUE": true, "PEER_EXCHANGE": true,
	"STORE_ARCHIVED_CONTRACT": true, "DELETE_ARCHIVED_CONTRACT": true, "ANNOTATE_ARCHIVED_CONTRACT": true,
	"KEY_SHREDDED": true, "APPLIED_SIGNATURE": true, "REVOKE_SIGNATURE": true,
	"PUBLISH_CONTRACT_TO_CATALOGUE": true, "ACTIVATE_CONTRACT": true,
}

// Feed fans each change out to every subscriber.
type Feed struct {
	mu   sync.Mutex
	subs map[chan Change]struct{}
}

func New() *Feed {
	return &Feed{subs: map[chan Change]struct{}{}}
}

// Subscribe returns the changes from now on and a function that ends the
// subscription. The channel is closed when the subscriber fell behind and lost
// a change; the subscriber then reloads what it shows.
func (f *Feed) Subscribe() (<-chan Change, func()) {
	ch := make(chan Change, 64)
	f.mu.Lock()
	f.subs[ch] = struct{}{}
	f.mu.Unlock()
	return ch, func() {
		f.mu.Lock()
		if _, ok := f.subs[ch]; ok {
			delete(f.subs, ch)
			close(ch)
		}
		f.mu.Unlock()
	}
}

// Publish passes a change to every subscriber. A subscriber with a full buffer
// loses the change, so its subscription ends (the channel is closed).
func (f *Feed) Publish(change Change) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for ch := range f.subs {
		select {
		case ch <- change:
		default:
			delete(f.subs, ch)
			close(ch)
		}
	}
}

// Handle publishes a bus event when it changes a contract.
func (f *Feed) Handle(evt cloudevent.Event) {
	if !changeEvents[evt.Type()] {
		return
	}
	var envelope struct {
		DID string `json:"did"`
	}
	if json.Unmarshal(evt.Data(), &envelope) != nil || envelope.DID == "" {
		return
	}
	f.Publish(Change{DID: envelope.DID, EventType: evt.Type()})
}

// Listen feeds the bus events into f. The subscription has no queue group, so
// every replica receives every event for the streams it serves.
func (f *Feed) Listen(sub *event.CloudEventSubClient) error {
	return sub.Subscribe(f.Handle)
}
