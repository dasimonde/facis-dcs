package service

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
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
// contracts, and compliance status. The compliance counts use the per-entry
// verdict of GET /archive/compliance (DCS-FR-CSA-19).
func (s *contractStorageArchivesrvc) Statistics(ctx context.Context, p *contractstoragearchive.StatisticsPayload) (*contractstoragearchive.ArchiveStatisticsResponse, error) {
	ctx, err := contractReadContext(ctx, s.DIDDocument)
	if err != nil {
		return nil, err
	}
	allowed, err := readableContractDIDs(ctx, s.DB)
	if err != nil {
		return nil, err
	}

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

	// The expiring-contracts look-ahead window is configurable via
	// DCS_ARCHIVE_EXPIRING_WINDOW_DAYS, default 30 days (DCS-FR-CSA-04).
	expiringWindow := conf.ArchiveExpiringWindow()
	res := &contractstoragearchive.ArchiveStatisticsResponse{
		RecentActions:      []*contractstoragearchive.ArchiveRecentAction{},
		ExpiringContracts:  []*contractstoragearchive.ArchiveExpiringContract{},
		ExpiringWindowDays: int(expiringWindow / (24 * time.Hour)),
	}

	metadata := make(map[string]db.ContractMetadata, len(archivedContracts))
	for _, contract := range archivedContracts {
		metadata[contract.DID] = contract
	}
	now := time.Now().UTC()
	distinct := map[string]bool{}
	for _, entry := range entries {
		res.StorageBytes += archiveEntryStorageBytes(entry)
		if entry.ArchiveStatus == "DELETED" {
			res.DeletedTotal++
			continue
		}
		res.ArchivedTotal++
		distinct[entry.DID] = true
		var meta *db.ContractMetadata
		if m, ok := metadata[entry.DID]; ok {
			meta = &m
		}
		if archiveEntryFlagged(archiveComplianceChecks(entry, meta, now)) {
			res.FlaggedTotal++
		} else {
			res.CompliantTotal++
		}
	}
	res.ArchivedContracts = len(distinct)

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

	res.RecentActions = recentArchiveActions(filterContractAuditChains(auditChains, allowed), archiveRecentActionLimit, p.Justification != nil)
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

// archiveEntryFlagged reports whether an entry fails at least one archive
// compliance rule (DCS-FR-CSA-19).
func archiveEntryFlagged(checks []*contractstoragearchive.ArchiveComplianceCheck) bool {
	for _, check := range checks {
		if !check.Passed {
			return true
		}
	}
	return false
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
// into the newest-first action list the dashboard shows. The actor of each
// action is audit-log content: it is shown only to a request that gives a
// justification (DCS-FR-PACM-04), which the access log records.
func recentArchiveActions(chains [][]datatype.AuditLogEntry, limit int, withActor bool) []*contractstoragearchive.ArchiveRecentAction {
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
		action := &contractstoragearchive.ArchiveRecentAction{
			OccurredAt: entry.CreatedAt.UTC().Format(time.RFC3339),
			EventType:  entry.EventType,
			Did:        did,
		}
		if withActor {
			actor := auditEntryActor(entry.EventData)
			action.Actor = &actor
		}
		actions = append(actions, action)
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
	// Synthetic evidence may contain several identities; only accept an unambiguous actor.
	actor := ""
	for key, value := range data {
		if !strings.HasSuffix(key, "_by") {
			continue
		}
		identity, ok := value.(string)
		if !ok || identity == "" {
			continue
		}
		if actor != "" && actor != identity {
			return ""
		}
		actor = identity
	}
	return actor
}
