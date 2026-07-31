package qry

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	event2 "digital-contracting-service/internal/processauditandcompliance/event"
)

// TrustGateDenialQry records one federation trust-gate rejection (ADR-19)
// against the contract the interaction concerned.
type TrustGateDenialQry struct {
	DID       string
	PeerDID   string
	Direction string
	Reason    string
}

// TrustGateDenialReporter persists a trust-gate denial in-process, the same
// transactional-outbox pattern IncidentReporter uses for POST /pac/report
// findings (queryincidentreport.go), so a rejected federation interaction is
// as auditable as a manually reported compliance finding.
type TrustGateDenialReporter struct {
	DB *sqlx.DB
}

func (h *TrustGateDenialReporter) Handle(ctx context.Context, query TrustGateDenialQry) error {
	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		_ = tx.Rollback()
	}(tx)

	if err := h.HandleTx(ctx, tx, query); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("could not commit transaction: %w", err)
	}
	return nil
}

// HandleTx is Handle's transaction-scoped half: it lets a caller that must
// also touch another table in the SAME commit (e.g. clearing a sync_fails
// retry entry atomically with the incident that terminates it)
// record the denial without a separate, independently-committing
// transaction.
func (h *TrustGateDenialReporter) HandleTx(ctx context.Context, tx *sqlx.Tx, query TrustGateDenialQry) error {
	if query.DID == "" {
		return errors.New("trust gate denial is missing a linked contract DID")
	}

	evt := event2.TrustGateDenialEvent{
		DID:        query.DID,
		PeerDID:    query.PeerDID,
		Direction:  query.Direction,
		RuleID:     "FEDERATION_TRUST_GATE_DENIAL",
		Severity:   "error",
		Reason:     query.Reason,
		OccurredAt: time.Now().UTC(),
	}
	if err := event.Create(ctx, tx, evt, componenttype.ProcessAuditAndCompliance); err != nil {
		return fmt.Errorf("could not create trust gate denial event: %w", err)
	}
	return nil
}

// HandleTxDeduped is HandleTx with an existence check for the exact same
// (DID, PeerDID, Direction) combination, performed in the same transaction
// immediately before the insert. A terminal policy-endpoint denial
// (PolicyFailure) can legitimately be reached more than once for the
// SAME underlying interaction: a single contract offer fires both an Offer
// and, once the PDF is regenerated, a PDF_REGENERATED event, each
// independently triggering a ship attempt a few hundred ms apart, and both
// hit the same terminal denial — only the first must raise an incident. The
// agreement-credential (layer 3a) path has its own, different dedup
// (sync_fails.gate_incident_recorded, a retry latch) and does not use this.
func (h *TrustGateDenialReporter) HandleTxDeduped(ctx context.Context, tx *sqlx.Tx, query TrustGateDenialQry) error {
	if query.DID == "" {
		return errors.New("trust gate denial is missing a linked contract DID")
	}

	var alreadyRecorded bool
	existsQuery := `
        SELECT EXISTS(
            SELECT 1 FROM outbox_events
            WHERE event_type = $1
              AND did = $2
              AND event_data ->> 'peer_did' = $3
              AND event_data ->> 'direction' = $4
        )
    `
	eventType := (event2.TrustGateDenialEvent{}).EventType()
	if err := tx.GetContext(ctx, &alreadyRecorded, existsQuery, eventType, query.DID, query.PeerDID, query.Direction); err != nil {
		return fmt.Errorf("could not check for an existing trust gate denial incident: %w", err)
	}
	if alreadyRecorded {
		return nil
	}
	return h.HandleTx(ctx, tx, query)
}
