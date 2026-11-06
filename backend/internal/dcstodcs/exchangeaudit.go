package dcstodcs

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype/componenttype"
	baseevent "digital-contracting-service/internal/base/event"
)

// Exchange directions of a PeerExchangeEvent.
const (
	ExchangeSent     = "SENT"
	ExchangeReceived = "RECEIVED"
)

// PeerExchangeEvent records one completed DCS-to-DCS exchange in the
// contract's audit trail (DCS-NFR-BR-08). It names the exchange, the
// direction and the authenticated counterparty; it carries no content.
type PeerExchangeEvent struct {
	DID        string    `json:"did"`
	Exchange   string    `json:"exchange"`
	Direction  string    `json:"direction"`
	PeerDID    string    `json:"peer_did"`
	OccurredAt time.Time `json:"occurred_at"`
}

// EventType implements [baseevent.Event].
func (e PeerExchangeEvent) EventType() string { return "PEER_EXCHANGE" }

// GetDID implements [baseevent.Event].
func (e PeerExchangeEvent) GetDID() string { return e.DID }

// RecordPeerExchangeTx writes the PEER_EXCHANGE audit event in the caller's
// transaction.
func RecordPeerExchangeTx(ctx context.Context, tx *sqlx.Tx, did, exchange, direction, peerDID string) error {
	return baseevent.Create(ctx, tx, PeerExchangeEvent{
		DID: did, Exchange: exchange, Direction: direction, PeerDID: peerDID, OccurredAt: time.Now().UTC(),
	}, componenttype.ContractWorkflowEngine)
}

// PeerExchangeRefusedEvent records a peer that refused a notice of this
// instance for good: the peer holds no copy of the contract shared with this
// instance. The notice is not sent again. The contract's lifecycle shows it.
type PeerExchangeRefusedEvent struct {
	DID        string    `json:"did"`
	Exchange   string    `json:"exchange"`
	PeerDID    string    `json:"peer_did"`
	Reason     string    `json:"reason"`
	OccurredAt time.Time `json:"occurred_at"`
}

// EventType implements [baseevent.Event].
func (e PeerExchangeRefusedEvent) EventType() string { return "PEER_EXCHANGE_REFUSED" }

// GetDID implements [baseevent.Event].
func (e PeerExchangeRefusedEvent) GetDID() string { return e.DID }

// RecordPeerExchange writes the PEER_EXCHANGE audit event in its own
// transaction.
func RecordPeerExchange(ctx context.Context, db *sqlx.DB, did, exchange, direction, peerDID string) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := RecordPeerExchangeTx(ctx, tx, did, exchange, direction, peerDID); err != nil {
		return err
	}
	return tx.Commit()
}
