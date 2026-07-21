package event

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/db"
	"digital-contracting-service/internal/base/ipfs"
	"digital-contracting-service/internal/base/tsa"

	"golang.org/x/sync/errgroup"
)

type OutboxProcessor struct {
	DB           *sqlx.DB
	IPFSClient   *ipfs.APIClient
	TSAClient    *tsa.APIClient
	ARepo        db.AuditTrailRepository
	CEPPubClient *CloudEventPubClient
}

func (j OutboxProcessor) Start(ctx context.Context) error {
	go j.startPublishingJob(ctx, conf.OutboxPublishTimeOut())
	go j.startProcessingJob(ctx, conf.OutboxProcessorTimeOut())
	go j.startTimestampingJob(ctx, conf.AuditCheckpointTimestampRetry())
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

func (j OutboxProcessor) startProcessingJob(ctx context.Context, interval time.Duration) {
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
			WHERE processed = FALSE AND dead_lettered_at IS NULL
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

		// Read-only lookup events (RETRIEVE_*/SEARCH_*) are operational traces that
		// every audit read filters out (see base.IsAuditVisibleEventType). They are
		// never surfaced from the tamper-evident chain, so they are marked processed
		// without being anchored — otherwise the high-frequency traces dominate the
		// batch and starve genuine audit-visible events.
		anchorable := make([]datatype.OutboxEvent, 0, len(events))
		for _, event := range events {
			if base.IsAuditVisibleEventType(event.EventType) {
				anchorable = append(anchorable, event)
				continue
			}
			if err := j.processUnanchoredEvent(ctx, event); err != nil {
				log.Printf("could not mark lookup event %d processed: %v", event.ID, err)
			}
		}

		if len(anchorable) == 0 {
			return nil
		}
		return j.anchorBatch(ctx, anchorable)
	}

	ticker := time.NewTicker(interval)
	for range ticker.C {
		if err := schedulerLogic(); err != nil {
			log.Printf("could not process outbox entries: %v", err)
		}
	}
}

// anchorBatch anchors one batch of audit-visible events and commits to it with
// a single Merkle checkpoint (base/datatype.AuditCheckpoint).
//
// Entries of the same resource stay a strict hash chain — each links to its
// predecessor's CID — but different resources are independent and are written
// concurrently. The batch as a whole is committed to by one root, chained to
// the previous checkpoint's root and timestamped once, instead of one TSA
// round-trip per event. An entry that cannot be written is left out of this
// checkpoint and retried in the next one; it no longer holds back the events
// behind it.
func (j OutboxProcessor) anchorBatch(ctx context.Context, events []datatype.OutboxEvent) error {
	heads, err := j.readChainHeads(ctx, events)
	if err != nil {
		return err
	}

	anchored, updatedHeads := j.writeEntries(ctx, events, heads)
	if len(anchored) == 0 {
		return errors.New("no audit entry of this batch could be written, retrying next tick")
	}

	// Deterministic batch order: the outbox sequence, not the order in which the
	// concurrent writes happened to finish.
	sort.Slice(anchored, func(a, b int) bool { return anchored[a].eventID < anchored[b].eventID })

	leafHashes := make([]string, 0, len(anchored))
	leafCIDs := make([]string, 0, len(anchored))
	eventIDs := make([]int64, 0, len(anchored))
	for _, entry := range anchored {
		leafHashes = append(leafHashes, entry.leafHash)
		leafCIDs = append(leafCIDs, entry.cid)
		eventIDs = append(eventIDs, entry.eventID)
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

	prevRoot, err := j.ARepo.ReadLatestCheckpointRoot(ctx, tx)
	if err != nil {
		return fmt.Errorf("could not read the previous checkpoint root: %w", err)
	}
	root, err := base.MerkleRoot(leafHashes)
	if err != nil {
		return fmt.Errorf("could not compute the checkpoint root: %w", err)
	}

	checkpoint := datatype.AuditCheckpoint{
		Root:       root,
		PrevRoot:   prevRoot,
		LeafHashes: leafHashes,
		LeafCIDs:   leafCIDs,
		CreatedAt:  time.Now().UTC(),
	}
	stored, err := j.IPFSClient.CreateFile(ctx, checkpoint)
	if err != nil {
		return fmt.Errorf("could not store checkpoint: %w", err)
	}
	if _, err := j.IPFSClient.FetchFile(stored.Identifier.Value); err != nil {
		return fmt.Errorf("checkpoint CID %s not resolvable after store: %w", stored.Identifier.Value, err)
	}

	// The root is immutable, so a TSA that is slow or down must not hold up the
	// trail: the checkpoint is recorded either way and startTimestampingJob
	// attaches the timestamp once the TSA answers.
	var tsaSignature *string
	if signature, err := j.timestampRoot(ctx, root); err != nil {
		log.Printf("checkpoint %s stored without a timestamp, retrying later: %v", root, err)
	} else {
		tsaSignature = &signature
	}

	seq, err := j.ARepo.AppendCheckpoint(ctx, tx, stored.Identifier.Value, root, prevRoot, len(leafHashes), tsaSignature)
	if err != nil {
		return fmt.Errorf("could not append checkpoint: %w", err)
	}

	if err := j.ARepo.AppendCheckpointLeaves(ctx, tx, seq, leafCIDs, leafHashes); err != nil {
		return fmt.Errorf("could not record the leaves of checkpoint %d: %w", seq, err)
	}

	for key, cid := range updatedHeads {
		if err := j.ARepo.UpdateLogCID(ctx, tx, key.component, key.did, &cid); err != nil {
			return fmt.Errorf("could not update log CID: %w", err)
		}
	}

	for _, id := range eventIDs {
		if err := db.UpdateOutboxEvent(ctx, tx, id); err != nil {
			return fmt.Errorf("could not update outbox event %d: %w", id, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("could not commit checkpoint %d: %w", seq, err)
	}
	log.Printf("anchored checkpoint %d: %d entries, root %s, timestamped=%t", seq, len(leafHashes), root, tsaSignature != nil)
	return nil
}

// chainKey identifies the per-resource hash chain an event belongs to. Events
// that carry no resource DID are anchored by the checkpoint alone.
type chainKey struct {
	component string
	did       string
}

func chainKeyFor(event datatype.OutboxEvent) (chainKey, bool) {
	if isResourceDID(event.DID) {
		return chainKey{component: event.Component, did: *event.DID}, true
	}
	if event.Component == componenttype.System.String() && event.DID != nil && len(*event.DID) > 1 {
		return chainKey{component: event.Component, did: *event.DID}, true
	}
	return chainKey{}, false
}

// readChainHeads reads the current head CID of every per-resource chain this
// batch touches, in one transaction.
func (j OutboxProcessor) readChainHeads(ctx context.Context, events []datatype.OutboxEvent) (map[chainKey]*string, error) {
	tx, err := j.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	heads := make(map[chainKey]*string)
	for _, event := range events {
		key, ok := chainKeyFor(event)
		if !ok {
			continue
		}
		if _, seen := heads[key]; seen {
			continue
		}
		head, err := j.ARepo.ReadLogCID(ctx, tx, key.component, key.did)
		if err != nil {
			return nil, fmt.Errorf("could not read log CID: %w", err)
		}
		heads[key] = head
	}
	return heads, tx.Commit()
}

type anchoredEntry struct {
	eventID  int64
	cid      string
	leafHash string
}

// writeEntries writes every entry of the batch to IPFS, one chain at a time but
// all chains concurrently, and reports what was written plus each touched
// chain's new head. A chain stops at its first failure — its later entries link
// to a predecessor that does not exist yet — while the other chains carry on.
func (j OutboxProcessor) writeEntries(ctx context.Context, events []datatype.OutboxEvent, heads map[chainKey]*string) ([]anchoredEntry, map[chainKey]string) {
	grouped := make(map[chainKey][]datatype.OutboxEvent)
	unchained := make([]datatype.OutboxEvent, 0)
	for _, event := range events {
		key, ok := chainKeyFor(event)
		if !ok {
			unchained = append(unchained, event)
			continue
		}
		grouped[key] = append(grouped[key], event)
	}

	var mu sync.Mutex
	anchored := make([]anchoredEntry, 0, len(events))
	updatedHeads := make(map[chainKey]string)

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(16)
	for key, chain := range grouped {
		g.Go(func() error {
			pred := heads[key]
			for _, event := range chain {
				entry, err := j.writeEntry(gctx, event, pred)
				if err != nil {
					j.recordAnchorFailure(ctx, event, err)
					break
				}
				pred = &entry.cid
				mu.Lock()
				anchored = append(anchored, entry)
				updatedHeads[key] = entry.cid
				mu.Unlock()
			}
			return nil
		})
	}
	for _, event := range unchained {
		g.Go(func() error {
			entry, err := j.writeEntry(gctx, event, nil)
			if err != nil {
				j.recordAnchorFailure(ctx, event, err)
				return nil
			}
			mu.Lock()
			anchored = append(anchored, entry)
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		log.Printf("could not write audit entries: %v", err)
	}

	return anchored, updatedHeads
}

// recordAnchorFailure counts the failed attempt and dead-letters the event once
// it has failed too often, so a permanently unanchorable event stops being
// retried on every tick and becomes visible instead of merely noisy. The count
// is only advisory for a transient failure: the event is retried until the
// budget runs out.
func (j OutboxProcessor) recordAnchorFailure(ctx context.Context, event datatype.OutboxEvent, cause error) {
	tx, err := j.DB.BeginTxx(ctx, nil)
	if err != nil {
		log.Printf("could not record the anchoring failure of event %d: %v", event.ID, err)
		return
	}
	deadLettered, err := db.RecordOutboxAnchorFailure(ctx, tx, event.ID, cause.Error(), conf.OutboxAnchorMaxAttempts())
	if err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", rollbackErr)
		}
		log.Printf("could not record the anchoring failure of event %d: %v", event.ID, err)
		return
	}
	if err := tx.Commit(); err != nil {
		log.Printf("could not commit the anchoring failure of event %d: %v", event.ID, err)
		return
	}

	if deadLettered {
		log.Printf("DEAD-LETTERED audit event %d (%s/%s) after %d failed anchoring attempts, it is NOT in the audit trail: %v",
			event.ID, event.Component, event.EventType, conf.OutboxAnchorMaxAttempts(), cause)
		return
	}
	log.Printf("could not anchor event %d (%s/%s), retrying next tick: %v",
		event.ID, event.Component, event.EventType, cause)
}

// writeEntry stores one audit entry and returns its CID and leaf hash. The leaf
// hash is taken over the exact bytes stored, so an auditor can refetch the entry
// and recompute its membership in the checkpoint.
func (j OutboxProcessor) writeEntry(ctx context.Context, event datatype.OutboxEvent, predCID *string) (anchoredEntry, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return anchoredEntry{}, fmt.Errorf("could not draw a blinding nonce for event %d: %w", event.ID, err)
	}

	entry := datatype.AuditLogEntry{
		ID:            event.ID,
		Component:     event.Component,
		EventType:     event.EventType,
		EventData:     event.EventData,
		DID:           event.DID,
		CreatedAt:     event.CreatedAt,
		ResLogPredCID: predCID,
		Nonce:         hex.EncodeToString(nonce),
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		return anchoredEntry{}, fmt.Errorf("could not encode entry for event %d: %w", event.ID, err)
	}

	stored, err := j.IPFSClient.CreateFile(ctx, entry)
	if err != nil {
		return anchoredEntry{}, fmt.Errorf("could not create IPFS file for event %d: %w", event.ID, err)
	}

	// Confirm the entry resolves through the read path before it becomes a chain
	// link. The tenant store is eventually consistent, so a CID CreateFile has
	// just returned is not always immediately retrievable; anchoring it early
	// lets a later audit read walk to a link it cannot yet fetch and fail the
	// whole trail with a "DataIdentifier not found".
	if _, err := j.IPFSClient.FetchFile(stored.Identifier.Value); err != nil {
		return anchoredEntry{}, fmt.Errorf("audit entry CID %s not resolvable after store for event %d: %w", stored.Identifier.Value, event.ID, err)
	}

	return anchoredEntry{eventID: event.ID, cid: stored.Identifier.Value, leafHash: base.MerkleLeafHash(raw)}, nil
}

// startTimestampingJob attaches a trusted timestamp to checkpoints that were
// anchored while the TSA was unavailable. Roots are immutable, so timestamping
// one later is sound: the timestamp attests that the root — and with it every
// entry it commits to — existed no later than the time it was issued.
func (j OutboxProcessor) startTimestampingJob(ctx context.Context, interval time.Duration) {
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

		pending, err := j.ARepo.ReadCheckpointsAwaitingTimestamp(ctx, tx, 50)
		if err != nil {
			return fmt.Errorf("could not read checkpoints awaiting a timestamp: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("could not commit transaction: %w", err)
		}

		for _, checkpoint := range pending {
			signature, err := j.timestampRoot(ctx, checkpoint.Root)
			if err != nil {
				return fmt.Errorf("could not timestamp checkpoint %d: %w", checkpoint.Seq, err)
			}
			updateTx, err := j.DB.BeginTxx(ctx, nil)
			if err != nil {
				return fmt.Errorf("could not start transaction: %w", err)
			}
			if err := j.ARepo.UpdateCheckpointTimestamp(ctx, updateTx, checkpoint.Seq, signature); err != nil {
				if rollbackErr := updateTx.Rollback(); rollbackErr != nil {
					log.Printf("could not rollback transaction: %v", rollbackErr)
				}
				return fmt.Errorf("could not store the timestamp of checkpoint %d: %w", checkpoint.Seq, err)
			}
			if err := updateTx.Commit(); err != nil {
				return fmt.Errorf("could not commit the timestamp of checkpoint %d: %w", checkpoint.Seq, err)
			}
			log.Printf("timestamped checkpoint %d", checkpoint.Seq)
		}
		return nil
	}

	ticker := time.NewTicker(interval)
	for range ticker.C {
		if err := schedulerLogic(); err != nil {
			log.Printf("could not timestamp pending checkpoints: %v", err)
		}
	}
}

// timestampRoot has the TSA timestamp a checkpoint root and verifies the
// receipt straight away, so a broken certificate chain is caught here rather
// than years later at an audit.
func (j OutboxProcessor) timestampRoot(ctx context.Context, root string) (string, error) {
	receipt, err := j.TSAClient.Timestamp(ctx, root)
	if err != nil {
		return "", fmt.Errorf("could not timestamp root %s: %w", root, err)
	}
	verified, err := j.TSAClient.Verify(receipt, root)
	if !verified {
		return "", fmt.Errorf("timestamp verification failed for root %s: %w", root, err)
	}
	return receipt, nil
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
