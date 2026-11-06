package event

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype/componenttype"
	baseevent "digital-contracting-service/internal/base/event"
)

// Directions of a SigningConflictEvent.
const (
	// SigningConflictReceived is recorded by the instance that refused the
	// peer's signed copy.
	SigningConflictReceived = "RECEIVED"
	// SigningConflictSent is recorded by the instance whose signed copy the
	// peer refused.
	SigningConflictSent = "SENT"
)

// SigningConflictEvent records a signed contract PDF that a peer shipped and
// the receiver refused, because the PDF does not extend the signed revision
// the receiver holds (ADR-38 §2). Two instances then hold signatures over the
// same revision. Both instances record the event, and the compliance monitor
// reports it as a SIGNING_CONFLICT risk.
type SigningConflictEvent struct {
	DID        string    `json:"did"`
	PeerDID    string    `json:"peer_did"`
	Direction  string    `json:"direction"`
	Reason     string    `json:"reason"`
	OccurredAt time.Time `json:"occurred_at"`
}

// EventType implements the Event interface.
func (e SigningConflictEvent) EventType() string { return "PAC_SIGNING_CONFLICT" }

// GetDID implements the Event interface.
func (e SigningConflictEvent) GetDID() string { return e.DID }

// RecordSigningConflictTx writes a SigningConflictEvent in the caller's
// transaction. A conflict already recorded for the same contract, peer and
// direction is not written again, so a repeated ship of the refused copy
// adds no second event.
func RecordSigningConflictTx(ctx context.Context, tx *sqlx.Tx, did, peerDID, direction, reason string) error {
	evt := SigningConflictEvent{DID: did, PeerDID: peerDID, Direction: direction, Reason: reason, OccurredAt: time.Now().UTC()}
	var recorded bool
	if err := tx.GetContext(ctx, &recorded, `
        SELECT EXISTS(
            SELECT 1 FROM outbox_events
            WHERE event_type = $1 AND did = $2
              AND event_data ->> 'peer_did' = $3
              AND event_data ->> 'direction' = $4
        )`, evt.EventType(), did, peerDID, direction); err != nil {
		return fmt.Errorf("could not check for a recorded signing conflict on %s: %w", did, err)
	}
	if recorded {
		return nil
	}
	return baseevent.Create(ctx, tx, evt, componenttype.ProcessAuditAndCompliance)
}
