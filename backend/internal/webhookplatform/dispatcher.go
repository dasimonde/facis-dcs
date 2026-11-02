package webhookplatform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/base/safehttp"
)

// WebhookPayload is the JSON body sent to a subscriber's callback URL.
type WebhookPayload struct {
	EventID       string    `json:"event_id"`
	CorrelationID string    `json:"correlation_id"`
	Event         string    `json:"event"`
	DID           string    `json:"did"`
	OccurredAt    time.Time `json:"occurred_at"`
	// Owner names who owns the contract (created_by and the responsible
	// parties), so a receiving flow can route a contract event to them. It is
	// absent for template events.
	Owner json.RawMessage `json:"owner,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}

// Audience reports whether the organization ownerDID may receive event about
// did. Events carry contract content, so delivery follows the contract read
// policy.
type Audience func(ctx context.Context, event, did, ownerDID string) bool

// Owner returns the owner of the contract did as a JSON object, or nil.
type Owner func(ctx context.Context, did string) json.RawMessage

// Dispatcher fans out events to the registered subscribers asynchronously.
type Dispatcher struct {
	subs       Subscriptions
	store      Deliveries
	audience   Audience
	owner      Owner
	httpClient *http.Client
}

// NewDispatcher creates a Dispatcher that reads subscribers from subs, delivers
// to those audience admits, adds the contract owner from owner (may be nil),
// and records outcomes in store. Callback URLs are
// caller-supplied, so the client refuses loopback, link-local and redirects.
func NewDispatcher(subs Subscriptions, store Deliveries, audience Audience, owner Owner) *Dispatcher {
	return &Dispatcher{
		subs:       subs,
		store:      store,
		audience:   audience,
		owner:      owner,
		httpClient: safehttp.Client(10*time.Second, safehttp.Policy{}),
	}
}

// Dispatch sends event to every subscriber registered for that event whose
// organization the audience admits. eventID identifies the event: an event
// that is dispatched already is not sent again.
func (d *Dispatcher) Dispatch(ctx context.Context, eventID, event, did string, data json.RawMessage) error {
	subs, err := d.subs.ByEvent(ctx, event)
	if err != nil {
		return fmt.Errorf("webhookplatform: load subscribers for %s: %w", event, err)
	}
	subs = slices.DeleteFunc(subs, func(sub Subscription) bool {
		return !d.audience(ctx, event, did, sub.OwnerDID)
	})
	if len(subs) == 0 {
		return nil
	}

	payload := WebhookPayload{
		EventID:       eventID,
		CorrelationID: uuid.New().String(),
		Event:         event,
		DID:           did,
		OccurredAt:    time.Now().UTC(),
		Data:          data,
	}
	if d.owner != nil && !strings.HasPrefix(event, "template.") {
		payload.Owner = d.owner(ctx, did)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("webhookplatform: marshal payload of %s: %w", event, err)
	}
	deliveries := make([]Delivery, 0, len(subs))
	for _, sub := range subs {
		deliveries = append(deliveries, Delivery{
			EventID: payload.EventID, CorrelationID: payload.CorrelationID, Event: event, DID: did,
			SubscriptionID: sub.ID, CallbackURL: sub.CallbackURL, OwnerDID: sub.OwnerDID, Payload: body,
		})
	}
	// The dispatch and every delivery are recorded together before the first
	// attempt, each due again after retryDelay(0). A replica that stops before
	// it records them leaves the event to the next JetStream delivery; one that
	// stops after leaves the deliveries to RetryDue. Delivery is at least once.
	ids, err := d.store.RecordDispatch(ctx, PendingCallback{
		EventID:       payload.EventID,
		CorrelationID: payload.CorrelationID,
		Event:         event,
		DID:           did,
		SentAt:        payload.OccurredAt,
	}, deliveries, time.Now().Add(retryDelay(0)))
	if err != nil {
		return fmt.Errorf("webhookplatform: %w", err)
	}
	for i, id := range ids {
		deliveries[i].ID = id
		go d.attempt(context.WithoutCancel(ctx), deliveries[i], subs[i].Secret)
	}
	return nil
}

// maxDeliveryAttempts ends a delivery the subscriber did not accept.
const maxDeliveryAttempts = 10

// retryDelay is the wait after attempt number attempts (0 is the first): one
// minute, doubled per attempt, at most one hour.
func retryDelay(attempts int) time.Duration {
	return min(time.Minute<<min(attempts, 6), time.Hour)
}

// RetryDue sends the deliveries that are due again: not accepted yet, or not
// attempted because the dispatching replica stopped. A delivery whose
// subscription is gone ends.
func (d *Dispatcher) RetryDue(ctx context.Context) {
	due, err := d.store.DueDeliveries(ctx, time.Now(), 20)
	if err != nil {
		log.Printf("webhookplatform: %v", err)
		return
	}
	for _, delivery := range due {
		sub, err := d.subs.ByID(ctx, delivery.SubscriptionID)
		if err != nil {
			log.Printf("webhookplatform: %v", err)
			continue
		}
		if sub == nil {
			if err := d.store.RecordAttempt(ctx, delivery.ID, 0, "the subscription was deleted", time.Time{}); err != nil {
				log.Printf("webhookplatform: %v", err)
			}
			continue
		}
		d.attempt(ctx, delivery, sub.Secret)
	}
}

// DispatchFromDCS is a convenience wrapper that translates a raw DCS NATS
// event type to its webhook event name before dispatching.
func (d *Dispatcher) DispatchFromDCS(ctx context.Context, eventID, dcsEventType, did string, data json.RawMessage) error {
	event, ok := DCSEventMap[dcsEventType]
	if !ok {
		return nil
	}
	return d.Dispatch(ctx, eventID, event, did, data)
}

// errUnreachable is the delivery error a subscriber reads. The transport
// error stays in the log: it would tell the subscriber about addresses and
// ports inside the deployment.
const errUnreachable = "the callback URL could not be reached"

// attempt posts the delivery's payload once, signed with secret, and records
// the outcome: a 2xx answer or the last allowed attempt ends the delivery,
// anything else makes it due again after retryDelay.
func (d *Dispatcher) attempt(ctx context.Context, delivery Delivery, secret string) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	statusCode, errText := d.post(ctx, delivery, secret)
	next := time.Time{}
	if (statusCode < 200 || statusCode > 299) && delivery.Attempts+1 < maxDeliveryAttempts {
		next = time.Now().Add(retryDelay(delivery.Attempts + 1))
	}
	if err := d.store.RecordAttempt(ctx, delivery.ID, statusCode, errText, next); err != nil {
		log.Printf("webhookplatform: %v", err)
	}
}

// post sends the payload and returns the HTTP status, or 0 and the error a
// subscriber may read.
func (d *Dispatcher) post(ctx context.Context, delivery Delivery, secret string) (int, string) {
	if err := ValidateCallbackURL(delivery.CallbackURL); err != nil {
		return 0, err.Error()
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	signature, err := Signature(secret, delivery.EventID, timestamp, delivery.Payload)
	if err != nil {
		log.Printf("webhookplatform: sign payload for %s: %v", delivery.CallbackURL, err)
		return 0, "the delivery could not be signed"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, delivery.CallbackURL, bytes.NewReader(delivery.Payload))
	if err != nil {
		log.Printf("webhookplatform: build request for %s: %v", delivery.CallbackURL, err)
		return 0, errUnreachable
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Correlation-ID", delivery.CorrelationID)
	req.Header.Set("X-Event-ID", delivery.EventID)
	// Standard Webhooks v1 (https://www.standardwebhooks.com/).
	req.Header.Set("webhook-id", delivery.EventID)
	req.Header.Set("webhook-timestamp", timestamp)
	req.Header.Set("webhook-signature", signature)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		log.Printf("webhookplatform: notify %s [%s]: %v", delivery.CallbackURL, delivery.Event, err)
		return 0, errUnreachable
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

	log.Printf("webhookplatform: notified %s [%s] → HTTP %d", delivery.CallbackURL, delivery.Event, resp.StatusCode)
	return resp.StatusCode, ""
}

// ErrInvalidCallbackURL refuses a callback URL a delivery may not be posted to.
var ErrInvalidCallbackURL = errors.New("callback_url must be an absolute https URL without credentials")

// ValidateCallbackURL accepts an absolute https URL without credentials. A
// delivery carries contract content (DCS-IR-CI-01), so plain http is accepted
// only for a host that did:web may be resolved from over http (loopback, or a
// host DCS_DIDWEB_INSECURE_HOSTS names, see identity.DIDWebSchemes) or a
// host:port DCS_WEBHOOK_INSECURE_HOSTS names.
func ValidateCallbackURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil {
		return ErrInvalidCallbackURL
	}
	if u.Scheme == "https" {
		return nil
	}
	if u.Scheme == "http" && (slices.Contains(identity.DIDWebSchemes(u.Host), "http") || insecureWebhookHost(u.Host)) {
		return nil
	}
	return ErrInvalidCallbackURL
}

func insecureWebhookHost(host string) bool {
	for _, allowed := range strings.Split(os.Getenv("DCS_WEBHOOK_INSECURE_HOSTS"), ",") {
		if allowed = strings.TrimSpace(allowed); allowed != "" && strings.EqualFold(allowed, host) {
			return true
		}
	}
	return false
}
