package event

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/db"
	"digital-contracting-service/internal/base/ipfs"
	"digital-contracting-service/internal/base/tsa"
)

type OutboxProcessor struct {
	DB           *sqlx.DB
	IPFSClient   *ipfs.APIClient
	TSAClient    *tsa.APIClient
	ARepo        db.AuditTrailRepository
	CEPPubClient *CloudEventPubClient
}

func (j OutboxProcessor) Start(ctx context.Context, origin string) error {
	go j.startPublishingJob(ctx, conf.OutboxPublishTimeOut())
	go j.startProcessingJob(ctx, conf.OutboxProcessorTimeOut(), origin)
	return nil
}

// startPublishingJob republishes outbox events on NATS on its own ticker,
// independent of startProcessingJob's TSA/IPFS anchoring: subscribers
// (webhookplatform, pdfgeneration, contractworkflowengine/deployevent's
// auto-deploy) only ever consume an event's JSON payload, never an
// anchor-derived value, so publishing must not wait behind the strictly
// sequential, network-bound anchoring of earlier events in the same
// backlog — that decoupling is why `published` is a separate flag from
// `processed`.
func (j OutboxProcessor) startPublishingJob(ctx context.Context, interval time.Duration) {
	schedulerLogic := func() error {
		tx, err := j.DB.BeginTxx(ctx, nil)
		if err != nil {
			return fmt.Errorf("could not start transaction: %w", err)
		}
		defer func(tx *sqlx.Tx) {
			if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
				log.Printf("could not rollback transaction: %v", err)
			}
		}(tx)

		rows, err := tx.QueryxContext(ctx, `
			SELECT id, component, event_type, event_data, did, created_at
			FROM outbox_events
			WHERE published = FALSE
			ORDER BY created_at ASC
			LIMIT 200
			FOR UPDATE SKIP LOCKED
		`)
		if err != nil {
			return fmt.Errorf("could not query unpublished outbox events: %w", err)
		}

		var events []datatype.OutboxEvent
		for rows.Next() {
			var event datatype.OutboxEvent
			if err := rows.StructScan(&event); err != nil {
				if closeErr := rows.Close(); closeErr != nil {
					return closeErr
				}
				return fmt.Errorf("could not scan event: %w", err)
			}
			events = append(events, event)
		}
		if err := rows.Close(); err != nil {
			return err
		}

		for _, event := range events {
			if err := j.CEPPubClient.Publish(event.Component, event.EventType, event.EventData); err != nil {
				log.Printf("could not publish event %d: %v", event.ID, err)
				continue
			}
			if err := db.MarkOutboxEventPublished(ctx, tx, event.ID); err != nil {
				return fmt.Errorf("could not mark event %d published: %w", event.ID, err)
			}
		}

		return tx.Commit()
	}

	ticker := time.NewTicker(interval)
	for range ticker.C {
		if err := schedulerLogic(); err != nil {
			log.Printf("could not publish outbox entries: %v", err)
		}
	}
}

func (j OutboxProcessor) startProcessingJob(ctx context.Context, interval time.Duration, origin string) {
	schedulerLogic := func() error {
		tx, err := j.DB.BeginTxx(ctx, nil)
		if err != nil {
			return fmt.Errorf("could not start transaction: %w", err)
		}
		defer func(tx *sqlx.Tx) {
			if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
				log.Printf("could not rollback transaction: %v", err)
			}
		}(tx)

		rows, err := tx.QueryxContext(ctx, `
			SELECT id, component, event_type, event_data, did, created_at
			FROM outbox_events
			WHERE processed = FALSE
			ORDER BY created_at ASC
			LIMIT 100
			FOR UPDATE SKIP LOCKED
		`)
		if err != nil {
			return fmt.Errorf("could not query outbox events: %w", err)
		}

		var events []datatype.OutboxEvent
		for rows.Next() {
			var event datatype.OutboxEvent
			if err := rows.StructScan(&event); err != nil {
				err := rows.Close()
				if err != nil {
					return err
				}
				return fmt.Errorf("could not scan event: %w", err)
			}
			events = append(events, event)
		}
		err = rows.Close()
		if err != nil {
			return err
		}

		err = tx.Commit()
		if err != nil {
			return fmt.Errorf("could not commit transaction: %w", err)
		}

		if len(events) > 0 {
			log.Println("process ", len(events), " events")
		}

		// Keep going past a failing event instead of returning: anchoring is
		// network-bound (TSA, IPFS) and a single transient failure at the head of
		// the backlog used to stall every event behind it for as long as it kept
		// failing, so a whole contract's trail could read empty. Each event is
		// anchored in its own transaction and stays unprocessed on failure, so it
		// is simply retried on the next tick, behind the ones that did succeed.
		for _, event := range events {
			if err := j.processEvent(ctx, event, origin); err != nil {
				log.Printf("could not process event %d (%s/%s), retrying next tick: %v",
					event.ID, event.Component, event.EventType, err)
			}
		}

		return nil
	}

	ticker := time.NewTicker(interval)
	for range ticker.C {
		if err := schedulerLogic(); err != nil {
			log.Printf("could not process outbox entries: %v", err)
		}
	}
}

// processEvent anchors one outbox event into the tamper-evident audit trail:
// it chains the entry to the previous CID (both per-resource and globally),
// has it timestamped by the TSA, verifies that timestamp immediately as a
// sanity check, writes the signed entry to IPFS, and only then marks the
// outbox row processed. Because each entry embeds the hash of its
// predecessor, retroactively modifying an already-anchored entry breaks the
// chain and is detectable. The event was already republished on NATS by the
// caller before this ran (see startProcessingJob).
func (j OutboxProcessor) processEvent(ctx context.Context, event datatype.OutboxEvent, origin string) error {
	// Read-only lookup events (RETRIEVE_*/SEARCH_*) are operational traces that
	// every audit read filters out (see base.IsAuditVisibleEventType and its use
	// in both the contract and PAC audit handlers). They are never surfaced from
	// the tamper-evident chain, yet each one costs a synchronous TSA round-trip
	// plus an IPFS write here. Under a full BDD run these high-frequency traces
	// dominate the outbox and starve genuine audit-visible events (e.g. EXPORT):
	// an event created at the tail of a scenario could take ~45s to anchor,
	// missing the ~30s audit poll window. They still get republished on NATS and
	// marked processed, but they are not anchored into the hash chain.
	if !base.IsAuditVisibleEventType(event.EventType) {
		return j.processUnanchoredEvent(ctx, event)
	}

	tx, err := j.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	globalLogPredCID, err := j.ARepo.ReadLogCID(ctx, tx, conf.GlobalAuditTrailName(), conf.GlobalAuditTrailName())
	if err != nil {
		return fmt.Errorf("could not read log CID: %w", err)
	}

	var resLogPredCID *string
	if isResourceDID(event.DID) {
		resLogPredCID, err = j.ARepo.ReadLogCID(ctx, tx, event.Component, *event.DID)
		if err != nil {
			return fmt.Errorf("could not read log CID: %w", err)
		}
	} else if event.Component == componenttype.System.String() {
		if event.DID != nil && len(*event.DID) > 1 {
			resLogPredCID, err = j.ARepo.ReadLogCID(ctx, tx, event.Component, *event.DID)
			if err != nil {
				return fmt.Errorf("could not read log CID: %w", err)
			}
		}
	}

	auditLogEntry := datatype.AuditLogEntry{
		ID:               event.ID,
		Component:        event.Component,
		EventType:        event.EventType,
		EventData:        event.EventData,
		DID:              event.DID,
		CreatedAt:        event.CreatedAt,
		ResLogPredCID:    resLogPredCID,
		GlobalLogPredCID: globalLogPredCID,
	}

	tsaResult, err := j.TSAClient.Timestamp(ctx, auditLogEntry)
	if err != nil {
		return fmt.Errorf("could not timestamp event %d: %w", event.ID, err)
	}

	signedAuditLogEntry := datatype.SignedAuditLogEntry{
		ID:            event.ID,
		AuditLogEntry: auditLogEntry,
		TsaSignature:  tsaResult,
	}

	// sanity check that our cert is ok
	isVerified, verifyErr := j.TSAClient.Verify(tsaResult, auditLogEntry)
	if !isVerified {
		return fmt.Errorf("timestamp verification failed for event %d: %w", event.ID, verifyErr)
	}
	log.Printf("timestamp verification succeeded for event %d", event.ID)

	result, err := j.IPFSClient.CreateFile(ctx, signedAuditLogEntry)
	if err != nil {
		return fmt.Errorf("could not create IPFS file for event %d: %w", event.ID, err)
	}

	// Confirm the entry resolves through the read path before persisting its CID
	// as the audit-trail head. The tenant store is eventually consistent, so a
	// CID CreateFile has just returned is not always immediately retrievable;
	// persisting it early lets a later audit read walk the chain to a head — or
	// a predecessor link — it cannot yet fetch and fail the whole trail with a
	// "DataIdentifier not found". Blocking here until the entry is resolvable
	// makes every anchored CID a safe chain link (mirrors apply.go's readback).
	if _, err := j.IPFSClient.FetchFile(result.Identifier.Value); err != nil {
		return fmt.Errorf("audit entry CID %s not resolvable after store for event %d: %w", result.Identifier.Value, event.ID, err)
	}

	if isResourceDID(event.DID) {
		if err = j.ARepo.UpdateLogCID(ctx, tx, event.Component, *event.DID, &result.Identifier.Value); err != nil {
			return fmt.Errorf("could not update log CID: %w", err)
		}
	} else if event.Component == componenttype.System.String() {
		if event.DID != nil && len(*event.DID) > 1 {
			if err = j.ARepo.UpdateLogCID(ctx, tx, event.Component, *event.DID, &result.Identifier.Value); err != nil {
				return fmt.Errorf("could not update log CID: %w", err)
			}
		}
	}

	if err = j.ARepo.UpdateLogCID(ctx, tx, conf.GlobalAuditTrailName(), conf.GlobalAuditTrailName(), &result.Identifier.Value); err != nil {
		return fmt.Errorf("could not update log CID: %w", err)
	}

	err = db.UpdateOutboxEvent(ctx, tx, event.ID)
	if err != nil {
		return fmt.Errorf("could not update outbox event: %w", err)
	}

	return tx.Commit()
}

// processUnanchoredEvent marks a read-only-lookup outbox event (see
// base.IsAuditVisibleEventType) processed without anchoring it into the
// tamper-evident audit trail. It was already republished on NATS by the
// caller before this ran (see startProcessingJob). Skipping the TSA/IPFS
// anchoring keeps the outbox from backing up under the high volume of
// lookup events.
func (j OutboxProcessor) processUnanchoredEvent(ctx context.Context, event datatype.OutboxEvent) error {
	tx, err := j.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	if err = db.UpdateOutboxEvent(ctx, tx, event.ID); err != nil {
		return fmt.Errorf("could not update outbox event: %w", err)
	}

	return tx.Commit()
}

func isResourceDID(did *string) bool {
	return did != nil && len(*did) > 1 && *did != "*"
}
