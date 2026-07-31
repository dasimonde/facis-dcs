package webhookplatform

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// WebhookPayload is the JSON body sent to a subscriber's callback URL.
type WebhookPayload struct {
	EventID       string          `json:"event_id"`
	CorrelationID string          `json:"correlation_id"`
	Event         string          `json:"event"`
	DID           string          `json:"did"`
	OccurredAt    time.Time       `json:"occurred_at"`
	Data          json.RawMessage `json:"data,omitempty"`
}

// Dispatcher fans out events to all registered subscribers asynchronously.
type Dispatcher struct {
	store      *SubscriptionStore
	httpClient *http.Client
}

// NewDispatcher creates a Dispatcher backed by the given store.
func NewDispatcher(store *SubscriptionStore) *Dispatcher {
	return &Dispatcher{
		store: store,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Dispatch sends event to every subscriber registered for that event.
func (d *Dispatcher) Dispatch(ctx context.Context, event, did string, data json.RawMessage) {
	subs := d.store.GetByEvent(event)
	if len(subs) == 0 {
		return
	}

	payload := WebhookPayload{
		EventID:       uuid.New().String(),
		CorrelationID: uuid.New().String(),
		Event:         event,
		DID:           did,
		OccurredAt:    time.Now().UTC(),
		Data:          data,
	}

	d.store.TrackPending(PendingCallback{
		EventID:       payload.EventID,
		CorrelationID: payload.CorrelationID,
		Event:         event,
		DID:           did,
		SentAt:        payload.OccurredAt,
	})

	for _, sub := range subs {
		go d.notify(sub, payload)
	}
}

// DispatchFromDCS is a convenience wrapper that translates a raw DCS NATS
// event type to its webhook event name before dispatching.
func (d *Dispatcher) DispatchFromDCS(ctx context.Context, dcsEventType, did string, data json.RawMessage) {
	event, ok := DCSEventMap[dcsEventType]
	if !ok {
		return
	}
	d.Dispatch(ctx, event, did, data)
}

func (d *Dispatcher) notify(sub Subscription, payload WebhookPayload) {
	delivery := Delivery{
		EventID:       payload.EventID,
		CorrelationID: payload.CorrelationID,
		Event:         payload.Event,
		DID:           payload.DID,
		CallbackURL:   sub.CallbackURL,
		DeliveredAt:   time.Now().UTC(),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("webhookplatform: marshal payload for %s: %v", sub.CallbackURL, err)
		delivery.Error = err.Error()
		d.store.AddDelivery(delivery)
		return
	}

	req, err := http.NewRequest(http.MethodPost, sub.CallbackURL, bytes.NewReader(body))
	if err != nil {
		log.Printf("webhookplatform: build request for %s: %v", sub.CallbackURL, err)
		delivery.Error = err.Error()
		d.store.AddDelivery(delivery)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Correlation-ID", payload.CorrelationID)
	req.Header.Set("X-Event-ID", payload.EventID)
	if sub.Secret != "" {
		req.Header.Set("Authorization", "Bearer "+sub.Secret)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		log.Printf("webhookplatform: notify %s [%s]: %v", sub.CallbackURL, payload.Event, err)
		delivery.Error = err.Error()
		d.store.AddDelivery(delivery)
		return
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Printf("webhookplatform: close response body: %v", err)
		}
	}(resp.Body)
	_, err = io.Copy(io.Discard, resp.Body)
	if err != nil {
		log.Printf("webhookplatform: read response body: %v", err)
	}

	delivery.StatusCode = resp.StatusCode
	d.store.AddDelivery(delivery)

	log.Printf("webhookplatform: notified %s [%s] → HTTP %d", sub.CallbackURL, payload.Event, resp.StatusCode)
}
