package base

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/db"
	"digital-contracting-service/internal/base/ipfs"

	"golang.org/x/sync/errgroup"
)

type AuditTrailReader struct {
	ARepo      db.AuditTrailRepository
	IPFSClient *ipfs.APIClient
	Artifacts  *artifactstore.Store
}

func (r AuditTrailReader) ReadAuditLogEntriesByComponentAndDID(ctx context.Context, tx *sqlx.Tx, componentType componenttype.ComponentType, did string) ([]datatype.AuditLogEntry, error) {

	cid, err := r.ARepo.ReadLogCID(ctx, tx, componentType.String(), did)
	if err != nil {
		return nil, err
	}

	logEntries := make([]datatype.AuditLogEntry, 0)
	if cid == nil {
		return logEntries, nil
	}

	currentCID := *cid
	for {
		result, err := r.IPFSClient.FetchFile(currentCID)
		if err != nil {
			return nil, fmt.Errorf("read body: %w", err)
		}
		logEntry, err := r.openAuditLogEntry(ctx, result.Data)
		if err != nil {
			return nil, fmt.Errorf("decode response: %w", err)
		}

		logEntries = append(logEntries, logEntry)

		if logEntry.ResLogPredCID == nil {
			break
		}

		currentCID = *logEntry.ResLogPredCID
	}

	return logEntries, nil
}

func (r AuditTrailReader) ReadAuditLogEntriesByComponent(ctx context.Context, tx *sqlx.Tx, componentType componenttype.ComponentType) ([][]datatype.AuditLogEntry, error) {

	cids, err := r.ARepo.ReadLogCIDs(ctx, tx, componentType.String())
	if err != nil {
		return nil, err
	}

	// Each per-DID chain must be walked sequentially (entries link to their
	// predecessor by CID), but the chains are independent of each other —
	// walk them concurrently. A component-wide audit over N resources is
	// otherwise O(total events) sequential IPFS round-trips and blows past
	// any sane request deadline once the trail has real history.
	nonNil := make([]string, 0, len(cids))
	for _, cid := range cids {
		if cid != nil {
			nonNil = append(nonNil, *cid)
		}
	}

	chains := make([][]datatype.AuditLogEntry, len(nonNil))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(128)
	for i, headCID := range nonNil {
		g.Go(func() error {
			logEntries := make([]datatype.AuditLogEntry, 0)
			currentCID := headCID
			for {
				if err := gctx.Err(); err != nil {
					return err
				}
				fetched, err := r.IPFSClient.FetchFile(currentCID)
				if err != nil {
					return fmt.Errorf("read body: %w", err)
				}
				logEntry, err := r.openAuditLogEntry(gctx, fetched.Data)
				if err != nil {
					return fmt.Errorf("decode response: %w", err)
				}

				logEntries = append(logEntries, logEntry)

				if logEntry.ResLogPredCID == nil {
					break
				}

				currentCID = *logEntry.ResLogPredCID
			}
			chains[i] = logEntries
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	return chains, nil
}

// ReadAllAuditLogEntries returns the whole trail, newest first, by walking the
// Merkle checkpoints backwards and reading the entries each one commits to.
// The checkpoints ARE the global order: within one, the batch order the root
// was computed over; across them, the chain of roots.
func (r AuditTrailReader) ReadAllAuditLogEntries(ctx context.Context, tx *sqlx.Tx) ([]datatype.AuditLogEntry, error) {

	checkpoints, err := r.ARepo.ReadCheckpoints(ctx, tx, conf.AuditCheckpointReadLimit())
	if err != nil {
		return nil, err
	}

	logEntries := make([]datatype.AuditLogEntry, 0)
	for _, record := range checkpoints {
		fetched, err := r.Artifacts.Get(ctx, r.Artifacts.InstanceScope(), record.CID)
		if err != nil {
			return nil, fmt.Errorf("read checkpoint %d: %w", record.Seq, err)
		}
		var checkpoint datatype.AuditCheckpoint
		if err := json.Unmarshal(fetched, &checkpoint); err != nil {
			return nil, fmt.Errorf("decode checkpoint %d: %w", record.Seq, err)
		}

		entries := make([]datatype.AuditLogEntry, len(checkpoint.LeafCIDs))
		g, gctx := errgroup.WithContext(ctx)
		g.SetLimit(32)
		for i, cid := range checkpoint.LeafCIDs {
			g.Go(func() error {
				if err := gctx.Err(); err != nil {
					return err
				}
				leaf, err := r.IPFSClient.FetchFile(cid)
				if err != nil {
					return fmt.Errorf("read entry %s of checkpoint %d: %w", cid, record.Seq, err)
				}
				entry, err := r.openAuditLogEntry(gctx, leaf.Data)
				if err != nil {
					return fmt.Errorf("decode entry %s of checkpoint %d: %w", cid, record.Seq, err)
				}
				entries[i] = entry
				return nil
			})
		}
		if err := g.Wait(); err != nil {
			return nil, err
		}
		for i := len(entries) - 1; i >= 0; i-- {
			logEntries = append(logEntries, entries[i])
		}
	}

	return logEntries, nil
}

// openAuditLogEntry decodes a stored entry and opens its private body: the
// plaintext header is returned as-is, EventData is decrypted under the entry's
// CEK scope. Once the scope is shredded, the body becomes the defined erased
// marker instead of an error — the header and the chain stay readable.
func (r AuditTrailReader) openAuditLogEntry(ctx context.Context, data []byte) (datatype.AuditLogEntry, error) {
	var logEntry datatype.AuditLogEntry
	if err := json.Unmarshal(data, &logEntry); err != nil {
		return datatype.AuditLogEntry{}, err
	}
	if logEntry.CEKScopeKind == "" || len(logEntry.EventData) == 0 {
		return logEntry, nil
	}
	var body []byte
	if err := json.Unmarshal(logEntry.EventData, &body); err != nil {
		return datatype.AuditLogEntry{}, fmt.Errorf("decode encrypted body of entry %d: %w", logEntry.ID, err)
	}
	scope := artifactstore.Scope{Kind: artifactstore.Kind(logEntry.CEKScopeKind), ID: logEntry.CEKScopeID}
	plaintext, err := r.Artifacts.Decrypt(ctx, scope, body)
	if err != nil {
		if artifactstore.IsShredded(err) {
			logEntry.EventData = datatype.ErasedEventData
			return logEntry, nil
		}
		return datatype.AuditLogEntry{}, fmt.Errorf("decrypt body of entry %d: %w", logEntry.ID, err)
	}
	logEntry.EventData = plaintext
	return logEntry, nil
}
