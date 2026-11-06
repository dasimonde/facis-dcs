package db

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
)

// Negotiation message kinds persisted on contract_negotiation_outbound.kind.
const (
	NegotiationMessageProposal = "PROPOSAL"
	NegotiationMessageAccept   = "ACCEPT"
	NegotiationMessageReject   = "REJECT"
	// NegotiationMessageWithdraw retracts a Proposal the author still owns
	// before any counterparty has accepted it.
	NegotiationMessageWithdraw = "WITHDRAW"
	// NegotiationMessageOfferAccepted tells the origin that the counterparty
	// opened the round (Accept offer). No contract_negotiations row is created;
	// outbound rows use OfferAcceptedSentinelNegotiationID.
	NegotiationMessageOfferAccepted = "OFFER_ACCEPTED"
)

// OfferAcceptedSentinelNegotiationID fills negotiation_id for OFFER_ACCEPTED
// rows (column is NOT NULL, but no domain negotiation exists for this kind).
const OfferAcceptedSentinelNegotiationID = "00000000-0000-0000-0000-000000000000"

// NegotiationOutboundMessage is one negotiation fact queued toward one peer.
// DeliveredAt stays nil until that peer has acknowledged durable persistence.
type NegotiationOutboundMessage struct {
	ID              string     `db:"id"`
	DID             string     `db:"did"`
	FromPeerDID     string     `db:"from_peer_did"`
	ToPeerDID       string     `db:"to_peer_did"`
	Kind            string     `db:"kind"`
	NegotiationID   string     `db:"negotiation_id"`
	DecisionID      *string    `db:"decision_id"`
	ContractVersion int        `db:"contract_version"`
	RecordedAt      time.Time  `db:"recorded_at"`
	DeliveredAt     *time.Time `db:"delivered_at"`
}

// NegotiationOutboundRepo is the outbound delivery queue for negotiation
// messages. Domain rows are written first; this queue records each pending ship
// so peer downtime does not fail the local create.
type NegotiationOutboundRepo interface {
	// Enqueue records (or refreshes) one undelivered outbound row for the
	// message fact toward toPeerDID. Retries keep the same id when the row
	// already exists.
	Enqueue(ctx context.Context, tx *sqlx.Tx, msg NegotiationOutboundMessage) (string, error)
	// CancelUndeliveredProposal drops an undelivered PROPOSAL row for this
	// negotiation toward toPeerDID so a following WITHDRAW is not raced by a
	// late PROPOSAL delivery.
	CancelUndeliveredProposal(ctx context.Context, tx *sqlx.Tx, negotiationID, toPeerDID string) error
	GetUndelivered(ctx context.Context, tx *sqlx.Tx, fromPeerDID string) ([]NegotiationOutboundMessage, error)
	GetUndeliveredForDID(ctx context.Context, tx *sqlx.Tx, fromPeerDID, did string) ([]NegotiationOutboundMessage, error)
	MarkDelivered(ctx context.Context, tx *sqlx.Tx, id string) error
}
