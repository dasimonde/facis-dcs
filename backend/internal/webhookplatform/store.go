package webhookplatform

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

// KnownEvents is the list of subscribable DCS contract lifecycle events.
var KnownEvents = []EventInfo{
	{Name: "contract.created", Description: "A new contract was created"},
	{Name: "contract.submitted", Description: "A contract was submitted for review"},
	{Name: "contract.approved", Description: "A contract was approved"},
	{Name: "contract.rejected", Description: "A contract was rejected"},
	{Name: "contract.negotiated", Description: "A contract entered negotiation"},
	{Name: "contract.terminated", Description: "A contract was terminated"},
	// DCS-FR-CSA-20 / DCS-FR-CWE-10: API-delivered expiry alert; emitted by
	// the contractworkflowengine expiry cron when it persists EXPIRED.
	{Name: "contract.expired", Description: "A contract passed its expiration date"},
	{Name: "template.created", Description: "A new contract template was created"},
	{Name: "template.approved", Description: "A contract template was approved"},
	// DCS-FR-TR-22: Template Users subscribe to these to learn that a
	// template they have used was updated or re-registered as a new
	// version; the payload carries the template DID to filter on.
	{Name: "template.updated", Description: "A contract template was updated"},
	{Name: "template.registered", Description: "A contract template version was registered (published)"},
	{Name: "template.deprecated", Description: "A contract template was deprecated (archived)"},
	// DCS-FR-PACM-02: the "alert" half of "flagged and reported in real-time
	// via dashboards and alerts". Emitted by the continuous-monitoring sweep
	// the first time it detects a given violation, not on every run — see
	// querymonitor.go's reconcile.
	{Name: "compliance.risk_detected", Description: "Continuous monitoring detected a compliance risk"},
}

// DCSEventMap maps internal NATS event types to webhook event names.
var DCSEventMap = map[string]string{
	"CREATE_CONTRACT":            "contract.created",
	"SUBMIT_CONTRACT":            "contract.submitted",
	"APPROVE_CONTRACT":           "contract.approved",
	"REJECT_CONTRACT":            "contract.rejected",
	"NEGOTIATE_CONTRACT":         "contract.negotiated",
	"TERMINATE_CONTRACT":         "contract.terminated",
	"CONTRACT_EXPIRED":           "contract.expired",
	"CREATE_CONTRACT_TEMPLATE":   "template.created",
	"APPROVE_CONTRACT_TEMPLATE":  "template.approved",
	"UPDATE_CONTRACT_TEMPLATE":   "template.updated",
	"REGISTER_CONTRACT_TEMPLATE": "template.registered",
	"ARCHIVE_CONTRACT_TEMPLATE":  "template.deprecated",
	"PAC_COMPLIANCE_RISK":        "compliance.risk_detected",
}

// EventInfo describes a subscribable event.
type EventInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Subscription represents a registered webhook.
type Subscription struct {
	ID          string    `json:"id"`
	Event       string    `json:"event"`
	CallbackURL string    `json:"callback_url"`
	Secret      string    `json:"secret,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// PendingCallback tracks an in-flight webhook dispatch awaiting a callback.
type PendingCallback struct {
	EventID       string
	CorrelationID string
	Event         string
	DID           string
	SentAt        time.Time
}

// Delivery is the recorded outcome of one webhook notification attempt —
// the observable behind GET /deliveries (monitoring and BDD assertions).
type Delivery struct {
	EventID       string    `json:"event_id"`
	CorrelationID string    `json:"correlation_id"`
	Event         string    `json:"event"`
	DID           string    `json:"did"`
	CallbackURL   string    `json:"callback_url"`
	StatusCode    int       `json:"status_code,omitempty"`
	Error         string    `json:"error,omitempty"`
	DeliveredAt   time.Time `json:"delivered_at"`
	Acknowledged  bool      `json:"acknowledged"`
}

// maxDeliveries bounds the in-memory delivery log (newest kept).
const maxDeliveries = 512

// SubscriptionStore is a thread-safe in-memory store for subscriptions,
// pending callbacks, and the recent-delivery log.
type SubscriptionStore struct {
	mu         sync.RWMutex
	subs       map[string][]Subscription  // event name → subscriptions
	pending    map[string]PendingCallback // event_id → pending
	deliveries []Delivery                 // newest last, capped at maxDeliveries
}

// NewSubscriptionStore returns an empty store.
func NewSubscriptionStore() *SubscriptionStore {
	return &SubscriptionStore{
		subs:    make(map[string][]Subscription),
		pending: make(map[string]PendingCallback),
	}
}

// Add registers a new webhook subscription and returns it with a generated ID.
func (s *SubscriptionStore) Add(event, callbackURL, secret string) Subscription {
	sub := Subscription{
		ID:          uuid.New().String(),
		Event:       event,
		CallbackURL: callbackURL,
		Secret:      secret,
		CreatedAt:   time.Now().UTC(),
	}
	s.mu.Lock()
	s.subs[event] = append(s.subs[event], sub)
	s.mu.Unlock()
	return sub
}

// Delete removes a subscription by ID. Returns false when not found.
func (s *SubscriptionStore) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for event, subs := range s.subs {
		for i, sub := range subs {
			if sub.ID == id {
				s.subs[event] = append(subs[:i], subs[i+1:]...)
				return true
			}
		}
	}
	return false
}

// GetByEvent returns a snapshot of all subscriptions for the given event.
func (s *SubscriptionStore) GetByEvent(event string) []Subscription {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Subscription(nil), s.subs[event]...)
}

// ListAll returns all registered subscriptions.
func (s *SubscriptionStore) ListAll() []Subscription {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var all []Subscription
	for _, subs := range s.subs {
		all = append(all, subs...)
	}
	return all
}

// TrackPending records an in-flight dispatch so its callback can be correlated.
func (s *SubscriptionStore) TrackPending(p PendingCallback) {
	s.mu.Lock()
	s.pending[p.EventID] = p
	s.mu.Unlock()
}

// ResolvePending looks up and removes a pending callback by event_id, and
// flips the matching delivery-log entries to acknowledged.
func (s *SubscriptionStore) ResolvePending(eventID string) (PendingCallback, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.pending[eventID]
	if ok {
		delete(s.pending, eventID)
		for i := range s.deliveries {
			if s.deliveries[i].EventID == eventID {
				s.deliveries[i].Acknowledged = true
			}
		}
	}
	return p, ok
}

// AddDelivery appends a notification outcome to the delivery log.
func (s *SubscriptionStore) AddDelivery(d Delivery) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deliveries = append(s.deliveries, d)
	if len(s.deliveries) > maxDeliveries {
		s.deliveries = s.deliveries[len(s.deliveries)-maxDeliveries:]
	}
}

// ListDeliveries returns a snapshot of the delivery log, newest last.
func (s *SubscriptionStore) ListDeliveries() []Delivery {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Delivery(nil), s.deliveries...)
}
