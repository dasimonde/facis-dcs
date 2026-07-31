package service

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	contractstoragearchive "digital-contracting-service/gen/contract_storage_archive"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/contractworkflowengine/db"
)

// archiveRecentActionLimit caps the dashboard's recent-actions list.
const archiveRecentActionLimit = 10

// Statistics implements the archive dashboard overview (DCS-FR-CSA-21):
// archived-contract statistics, recent actions, storage volume, expiring
// contracts, and compliance status. An entry is compliant when its evidence
// is complete — snapshot, content hash, signature metadata, and TSA receipt
// (DCS-FR-CSA-19's completeness dimension); everything else is flagged.
func (s *contractStorageArchivesrvc) Statistics(ctx context.Context, _ *contractstoragearchive.StatisticsPayload) (*contractstoragearchive.ArchiveStatisticsResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()

	entries, err := s.CRepo.ReadArchiveEntries(ctx, tx)
	if err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	archivedContracts, err := s.CRepo.ReadArchivedContracts(ctx, tx)
	if err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	auditChains, err := s.ATrailReader.ReadAuditLogEntriesByComponent(ctx, tx, componenttype.ContractStorageArchive)
	if err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}

	res := &contractstoragearchive.ArchiveStatisticsResponse{
		RecentActions:     []*contractstoragearchive.ArchiveRecentAction{},
		ExpiringContracts: []*contractstoragearchive.ArchiveExpiringContract{},
	}

	distinct := map[string]bool{}
	for _, entry := range entries {
		res.StorageBytes += archiveEntryStorageBytes(entry)
		if entry.ArchiveStatus == "DELETED" {
			res.DeletedTotal++
			continue
		}
		res.ArchivedTotal++
		distinct[entry.DID] = true
		if archiveEntryEvidenceComplete(entry) {
			res.CompliantTotal++
		} else {
			res.FlaggedTotal++
		}
	}
	res.ArchivedContracts = len(distinct)

	// The expiring-contracts look-ahead window is configurable via
	// DCS_ARCHIVE_EXPIRING_WINDOW_DAYS, default 30 days (DCS-FR-CSA-04).
	expiringWindow := conf.ArchiveExpiringWindow()
	now := time.Now().UTC()
	for _, contract := range archivedContracts {
		if contract.ExpDate == nil {
			continue
		}
		expires := contract.ExpDate.UTC()
		if expires.Before(now) || expires.After(now.Add(expiringWindow)) {
			continue
		}
		res.ExpiringContracts = append(res.ExpiringContracts, &contractstoragearchive.ArchiveExpiringContract{
			Did:     contract.DID,
			Name:    contract.Name,
			ExpDate: expires.Format(time.RFC3339),
		})
	}
	sort.Slice(res.ExpiringContracts, func(i, j int) bool {
		return res.ExpiringContracts[i].ExpDate < res.ExpiringContracts[j].ExpDate
	})

	res.RecentActions = recentArchiveActions(auditChains, archiveRecentActionLimit)
	return res, nil
}

// archiveEntryStorageBytes measures the archived footprint of one entry: the
// contract snapshot plus every evidence document stored beside it.
func archiveEntryStorageBytes(entry db.ContractArchiveEntry) int64 {
	total := int64(len(entry.ContractSnapshot))
	for _, blob := range []*datatype.JSON{entry.SignatureMeta, entry.CredentialHashes, entry.TSAReceipt, entry.Evidence} {
		if blob != nil {
			total += int64(len(*blob))
		}
	}
	return total
}

// archiveEntryEvidenceComplete reports whether the entry carries the full
// evidence set an archived signed contract must hold.
func archiveEntryEvidenceComplete(entry db.ContractArchiveEntry) bool {
	return entry.ContentHash != "" &&
		entry.ContractSnapshot.IsNotNullValue() &&
		archiveEvidencePresent(entry.SignatureMeta) &&
		archiveEvidencePresent(entry.TSAReceipt)
}

// archiveEvidencePresent reports whether an evidence document holds content
// beyond the schema's empty-object default.
func archiveEvidencePresent(blob *datatype.JSON) bool {
	if blob == nil {
		return false
	}
	var data map[string]any
	if err := json.Unmarshal(*blob, &data); err != nil {
		return false
	}
	return len(data) > 0
}

// recentArchiveActions flattens the component's per-resource audit chains
// into the newest-first action list the dashboard shows.
func recentArchiveActions(chains [][]datatype.AuditLogEntry, limit int) []*contractstoragearchive.ArchiveRecentAction {
	flat := []datatype.AuditLogEntry{}
	for _, chain := range chains {
		flat = append(flat, chain...)
	}
	sort.Slice(flat, func(i, j int) bool { return flat[i].CreatedAt.After(flat[j].CreatedAt) })
	if len(flat) > limit {
		flat = flat[:limit]
	}
	actions := make([]*contractstoragearchive.ArchiveRecentAction, 0, len(flat))
	for _, entry := range flat {
		did := ""
		if entry.DID != nil {
			did = *entry.DID
		}
		actions = append(actions, &contractstoragearchive.ArchiveRecentAction{
			Actor:      auditEntryActor(entry.EventData),
			OccurredAt: entry.CreatedAt.UTC().Format(time.RFC3339),
			EventType:  entry.EventType,
			Did:        did,
		})
	}
	return actions
}

// auditEntryActor extracts the acting identity from an audit event's data —
// the archive events name their actor per operation (stored_by,
// retrieved_by, deleted_by, …).
func auditEntryActor(raw json.RawMessage) string {
	var data map[string]any
	if json.Unmarshal(raw, &data) != nil {
		return ""
	}
	for _, key := range []string{"actor", "stored_by", "retrieved_by", "searched_by", "deleted_by", "annotated_by", "applied_by", "created_by"} {
		if actor, ok := data[key].(string); ok && actor != "" {
			return actor
		}
	}
	return ""
}
