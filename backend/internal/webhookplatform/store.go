package webhookplatform

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
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
	// DCS-FR-CWE-22 / DCS-FR-CSA-20: renewal-deadline alert; emitted by the
	// expiry cron once per expiry date when the notice period starts.
	{Name: "contract.renewal_due", Description: "The notice period before a contract's expiration date started"},
	// DCS-FR-CSA-23: the other state changes a contract owner follows.
	{Name: "contract.offered", Description: "A contract was offered to the counterparty"},
	{Name: "contract.withdrawn", Description: "A contract offer was withdrawn"},
	{Name: "contract.signature_applied", Description: "A signature was applied to a contract"},
	{Name: "contract.signature_revoked", Description: "A contract signature was revoked"},
	{Name: "contract.activated", Description: "The target system acknowledged the deployment and the contract is active"},
	{Name: "contract.renewed", Description: "A contract was renewed"},
	{Name: "template.created", Description: "A new contract template was created"},
	{Name: "template.approved", Description: "A contract template was approved"},
	// DCS-FR-TR-22: Template Users subscribe to these to learn that a
	// template they have used was updated or re-registered as a new
	// version; the payload carries the template DID to filter on.
	{Name: "template.updated", Description: "A contract template was updated"},
	{Name: "template.registered", Description: "A contract template version was registered (published)"},
	{Name: "template.deprecated", Description: "A contract template was deprecated (archived)"},
	{Name: "template.deleted", Description: "A contract template was deleted"},
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
	"CONTRACT_RENEWAL_DUE":       "contract.renewal_due",
	"OFFER_CONTRACT":             "contract.offered",
	"WITHDRAW_CONTRACT":          "contract.withdrawn",
	"APPLIED_SIGNATURE":          "contract.signature_applied",
	"REVOKE_SIGNATURE":           "contract.signature_revoked",
	"ACTIVATE_CONTRACT":          "contract.activated",
	"RENEW_CONTRACT":             "contract.renewed",
	"CREATE_CONTRACT_TEMPLATE":   "template.created",
	"APPROVE_CONTRACT_TEMPLATE":  "template.approved",
	"UPDATE_CONTRACT_TEMPLATE":   "template.updated",
	"REGISTER_CONTRACT_TEMPLATE": "template.registered",
	"ARCHIVE_CONTRACT_TEMPLATE":  "template.deprecated",
	"DELETE_CONTRACT_TEMPLATE":   "template.deleted",
	"PAC_COMPLIANCE_RISK":        "compliance.risk_detected",
}

// EventInfo describes a subscribable event.
type EventInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Subscription represents a registered webhook.
//
// Secret is the plaintext signing secret. It exists only in memory: Postgres
// holds it encrypted, and only the POST /webhooks response carries it.
type Subscription struct {
	ID          string    `db:"id"`
	Event       string    `db:"event"`
	CallbackURL string    `db:"callback_url"`
	Secret      string    `db:"-"`
	OwnerDID    string    `db:"owner_did"`
	CreatedBy   string    `db:"created_by"`
	CreatedAt   time.Time `db:"created_at"`
}

// Subscriptions stores webhook subscriptions. Every caller-facing operation is
// scoped to the owning organization; ByEvent serves delivery only.
type Subscriptions interface {
	Add(ctx context.Context, sub Subscription) (Subscription, error)
	ListByOwner(ctx context.Context, ownerDID string) ([]Subscription, error)
	// DeleteOwned returns false when no subscription with that id belongs to ownerDID.
	DeleteOwned(ctx context.Context, id, ownerDID string) (bool, error)
	ByEvent(ctx context.Context, event string) ([]Subscription, error)
	// ByID returns the subscription with its secret, or nil when it is gone.
	ByID(ctx context.Context, id string) (*Subscription, error)
}

// PostgresSubscriptions stores subscriptions in webhook_subscriptions, with
// the secret encrypted by Secrets.
type PostgresSubscriptions struct {
	DB      *sqlx.DB
	Secrets SecretBox
}

const subscriptionColumns = `id, event, callback_url, owner_did, created_by, created_at`

func (s PostgresSubscriptions) Add(ctx context.Context, sub Subscription) (Subscription, error) {
	// The id is drawn here because the ciphertext is bound to it.
	id := uuid.New().String()
	ciphertext, wrappedCEK, err := s.Secrets.Seal(id, sub.Secret)
	if err != nil {
		return Subscription{}, err
	}
	var created Subscription
	err = s.DB.GetContext(ctx, &created, `
        INSERT INTO webhook_subscriptions (id, event, callback_url, secret_ciphertext, secret_wrapped_cek, owner_did, created_by)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
        RETURNING `+subscriptionColumns,
		id, sub.Event, sub.CallbackURL, ciphertext, wrappedCEK, sub.OwnerDID, sub.CreatedBy)
	if err != nil {
		return Subscription{}, fmt.Errorf("create webhook subscription: %w", err)
	}
	created.Secret = sub.Secret
	return created, nil
}

func (s PostgresSubscriptions) ListByOwner(ctx context.Context, ownerDID string) ([]Subscription, error) {
	subs := []Subscription{}
	if err := s.DB.SelectContext(ctx, &subs, `
        SELECT `+subscriptionColumns+` FROM webhook_subscriptions
        WHERE owner_did = $1 ORDER BY created_at, id`, ownerDID); err != nil {
		return nil, fmt.Errorf("list webhook subscriptions: %w", err)
	}
	return subs, nil
}

func (s PostgresSubscriptions) DeleteOwned(ctx context.Context, id, ownerDID string) (bool, error) {
	if _, err := uuid.Parse(id); err != nil {
		return false, nil
	}
	res, err := s.DB.ExecContext(ctx, `DELETE FROM webhook_subscriptions WHERE id = $1 AND owner_did = $2`, id, ownerDID)
	if err != nil {
		return false, fmt.Errorf("delete webhook subscription %s: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("delete webhook subscription %s: %w", id, err)
	}
	return n > 0, nil
}

// ByEvent returns the subscriptions with their decrypted secrets.
func (s PostgresSubscriptions) ByEvent(ctx context.Context, event string) ([]Subscription, error) {
	var rows []struct {
		Subscription
		Ciphertext []byte `db:"secret_ciphertext"`
		WrappedCEK []byte `db:"secret_wrapped_cek"`
	}
	if err := s.DB.SelectContext(ctx, &rows, `
        SELECT `+subscriptionColumns+`, secret_ciphertext, secret_wrapped_cek
        FROM webhook_subscriptions WHERE event = $1`, event); err != nil {
		return nil, fmt.Errorf("list webhook subscriptions for %s: %w", event, err)
	}
	subs := make([]Subscription, 0, len(rows))
	for _, row := range rows {
		// One unreadable secret must not stop delivery to the other subscribers.
		secret, err := s.Secrets.Open(row.ID, row.Ciphertext, row.WrappedCEK)
		if err != nil {
			log.Printf("webhookplatform: skip subscription %s: %v", row.ID, err)
			continue
		}
		row.Secret = secret
		subs = append(subs, row.Subscription)
	}
	return subs, nil
}

// ByID returns the subscription with its decrypted secret, or nil.
func (s PostgresSubscriptions) ByID(ctx context.Context, id string) (*Subscription, error) {
	var row struct {
		Subscription
		Ciphertext []byte `db:"secret_ciphertext"`
		WrappedCEK []byte `db:"secret_wrapped_cek"`
	}
	err := s.DB.GetContext(ctx, &row, `
        SELECT `+subscriptionColumns+`, secret_ciphertext, secret_wrapped_cek
        FROM webhook_subscriptions WHERE id = $1`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read webhook subscription %s: %w", id, err)
	}
	if row.Secret, err = s.Secrets.Open(row.ID, row.Ciphertext, row.WrappedCEK); err != nil {
		return nil, err
	}
	return &row.Subscription, nil
}

// PendingCallback tracks an in-flight webhook dispatch awaiting a callback.
type PendingCallback struct {
	EventID       string
	CorrelationID string
	Event         string
	DID           string
	SentAt        time.Time
}

// Delivery is the delivery of one dispatched event to one subscription and
// the outcome of its last attempt — the observable behind GET /deliveries
// (monitoring and BDD assertions).
type Delivery struct {
	ID             int64     `json:"-" db:"id"`
	EventID        string    `json:"event_id" db:"event_id"`
	CorrelationID  string    `json:"correlation_id" db:"correlation_id"`
	Event          string    `json:"event" db:"event"`
	DID            string    `json:"did" db:"did"`
	SubscriptionID string    `json:"-" db:"subscription_id"`
	CallbackURL    string    `json:"callback_url" db:"callback_url"`
	OwnerDID       string    `json:"-" db:"owner_did"`
	StatusCode     int       `json:"status_code,omitempty" db:"status_code"`
	Error          string    `json:"error,omitempty" db:"error"`
	Attempts       int       `json:"attempts" db:"attempts"`
	DeliveredAt    time.Time `json:"delivered_at" db:"delivered_at"`
	Acknowledged   bool      `json:"acknowledged" db:"acknowledged"`
	// Payload is the signed body, kept until the delivery ends.
	Payload []byte `json:"-" db:"payload"`
}

// maxDeliveries bounds how many entries ListDeliveries returns (newest kept).
const maxDeliveries = 512

// Deliveries stores the dispatches, their deliveries and the outcomes.
type Deliveries interface {
	// RecordDispatch records the dispatch of an event and its delivery to
	// each subscription in one transaction, every delivery due again at
	// retryAt, and returns the delivery ids in the order given. It returns
	// nil when a dispatch of p.EventID is recorded already: its deliveries
	// exist and RetryDue sends them.
	RecordDispatch(ctx context.Context, p PendingCallback, deliveries []Delivery, retryAt time.Time) ([]int64, error)
	// RecordAttempt records the outcome of one attempt. The delivery is due
	// again at next; a zero next ends it.
	RecordAttempt(ctx context.Context, id int64, statusCode int, errText string, next time.Time) error
	// DueDeliveries returns up to limit deliveries due at now.
	DueDeliveries(ctx context.Context, now time.Time, limit int) ([]Delivery, error)
	// ResolvePending acknowledges the deliveries of eventID to ownerDID's
	// subscriptions and returns the dispatch. Only the first callback of an
	// organization for an event_id resolves it.
	ResolvePending(ctx context.Context, eventID, ownerDID string) (PendingCallback, bool, error)
	// ListDeliveries returns the newest deliveries to ownerDID's subscriptions,
	// newest last.
	ListDeliveries(ctx context.Context, ownerDID string) ([]Delivery, error)
}

// PostgresDeliveries stores pending callbacks and the delivery log in
// webhook_dispatches and webhook_deliveries, so every replica of the instance
// sees the same state (DCS-NFR-PER-02).
type PostgresDeliveries struct {
	DB *sqlx.DB
}

func (s PostgresDeliveries) RecordDispatch(ctx context.Context, p PendingCallback, deliveries []Delivery, retryAt time.Time) ([]int64, error) {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("record webhook dispatch %s: %w", p.EventID, err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `
        INSERT INTO webhook_dispatches (event_id, correlation_id, event, did, sent_at)
        VALUES ($1, $2, $3, $4, $5)
        ON CONFLICT (event_id) DO NOTHING`,
		p.EventID, p.CorrelationID, p.Event, p.DID, p.SentAt)
	if err != nil {
		return nil, fmt.Errorf("record webhook dispatch %s: %w", p.EventID, err)
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return nil, err
	}
	ids := make([]int64, 0, len(deliveries))
	for _, d := range deliveries {
		var id int64
		if err := tx.GetContext(ctx, &id, `
            INSERT INTO webhook_deliveries (event_id, subscription_id, callback_url, owner_did, payload, next_attempt_at)
            VALUES ($1, NULLIF($2, '')::uuid, $3, $4, $5, $6) RETURNING id`,
			p.EventID, d.SubscriptionID, d.CallbackURL, d.OwnerDID, d.Payload, retryAt); err != nil {
			return nil, fmt.Errorf("queue webhook delivery %s: %w", p.EventID, err)
		}
		ids = append(ids, id)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("record webhook dispatch %s: %w", p.EventID, err)
	}
	return ids, nil
}

func (s PostgresDeliveries) RecordAttempt(ctx context.Context, id int64, statusCode int, errText string, next time.Time) error {
	var nextAttempt *time.Time
	if !next.IsZero() {
		nextAttempt = &next
	}
	if _, err := s.DB.ExecContext(ctx, `
        UPDATE webhook_deliveries
        SET status_code = $2, error = $3, attempts = attempts + 1, delivered_at = now(), next_attempt_at = $4,
            payload = CASE WHEN $4::timestamptz IS NULL THEN NULL ELSE payload END
        WHERE id = $1`, id, statusCode, errText, nextAttempt); err != nil {
		return fmt.Errorf("record webhook delivery attempt %d: %w", id, err)
	}
	return nil
}

func (s PostgresDeliveries) DueDeliveries(ctx context.Context, now time.Time, limit int) ([]Delivery, error) {
	out := []Delivery{}
	if err := s.DB.SelectContext(ctx, &out, `
        SELECT d.id, d.event_id, p.correlation_id, p.event, p.did, COALESCE(d.subscription_id::text, '') AS subscription_id,
               d.callback_url, d.owner_did, d.status_code, d.error, d.attempts, COALESCE(d.delivered_at, p.sent_at) AS delivered_at,
               d.acknowledged_at IS NOT NULL AS acknowledged, d.payload
        FROM webhook_deliveries d JOIN webhook_dispatches p ON p.event_id = d.event_id
        WHERE d.next_attempt_at <= $1
        ORDER BY d.next_attempt_at LIMIT $2`, now, limit); err != nil {
		return nil, fmt.Errorf("list due webhook deliveries: %w", err)
	}
	return out, nil
}

func (s PostgresDeliveries) ResolvePending(ctx context.Context, eventID, ownerDID string) (PendingCallback, bool, error) {
	var p PendingCallback
	err := s.DB.QueryRowxContext(ctx, `
        WITH acknowledged AS (
            UPDATE webhook_deliveries SET acknowledged_at = now()
            WHERE event_id = $1 AND owner_did = $2 AND acknowledged_at IS NULL
            RETURNING event_id
        )
        SELECT event_id, correlation_id, event, did, sent_at FROM webhook_dispatches
        WHERE event_id IN (SELECT event_id FROM acknowledged)`, eventID, ownerDID).
		Scan(&p.EventID, &p.CorrelationID, &p.Event, &p.DID, &p.SentAt)
	if errors.Is(err, sql.ErrNoRows) {
		return PendingCallback{}, false, nil
	}
	if err != nil {
		return PendingCallback{}, false, fmt.Errorf("resolve webhook dispatch %s: %w", eventID, err)
	}
	return p, true, nil
}

func (s PostgresDeliveries) ListDeliveries(ctx context.Context, ownerDID string) ([]Delivery, error) {
	out := []Delivery{}
	if err := s.DB.SelectContext(ctx, &out, `
        SELECT event_id, correlation_id, event, did, callback_url, owner_did, status_code, error, attempts, delivered_at, acknowledged
        FROM (
            SELECT d.id, d.event_id, p.correlation_id, p.event, p.did, d.callback_url, d.owner_did,
                   d.status_code, d.error, d.attempts, COALESCE(d.delivered_at, p.sent_at) AS delivered_at,
                   d.acknowledged_at IS NOT NULL AS acknowledged
            FROM webhook_deliveries d JOIN webhook_dispatches p ON p.event_id = d.event_id
            WHERE d.owner_did = $1
            ORDER BY d.id DESC LIMIT $2
        ) newest ORDER BY id`, ownerDID, maxDeliveries); err != nil {
		return nil, fmt.Errorf("list webhook deliveries: %w", err)
	}
	return out, nil
}
