package service

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	processauditandcompliance "digital-contracting-service/gen/process_audit_and_compliance"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/contractworkflowengine/db"
)

// auditArchiveTrailEntries returns, per archived DID, the entry summaries and
// the integrity checks of the readable archive entries. A non-empty did keeps
// that DID only, before any check runs.
func (s *processAuditAndCompliancesrvc) auditArchiveTrailEntries(ctx context.Context, did string) (map[string][]*processauditandcompliance.PACResourceAuditTrailEntry, error) {
	result := map[string][]*processauditandcompliance.PACResourceAuditTrailEntry{}
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	entries, err := s.CRepo.ReadArchiveEntries(ctx, tx)
	if err != nil {
		return nil, err
	}
	if did != "" {
		entries = slices.DeleteFunc(entries, func(entry db.ContractArchiveEntry) bool { return entry.DID != did })
	}
	if len(entries) == 0 {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return result, nil
	}

	notaryEvents := map[string][]archiveNotaryEvent{}
	var chainErr error
	chainReader, err := newArchiveNotaryChainReaderFromEnv()
	if err != nil {
		chainErr = err
	} else if notaryEvents, err = chainReader.Read(ctx); err != nil {
		chainErr = err
	}

	for i, entry := range entries {
		did := entry.DID
		result[did] = append(result[did], &processauditandcompliance.PACResourceAuditTrailEntry{
			ID:        int64(-5000000 - i),
			Component: componenttype.ContractStorageArchive.String(),
			EventType: "ARCHIVE_ENTRY_AUDIT_SUMMARY",
			EventData: map[string]any{
				"objectType":           "contractArchiveEntry",
				"objectDid":            entry.DID,
				"contractVersion":      entry.ContractVersion,
				"archiveStatus":        entry.ArchiveStatus,
				"storedBy":             entry.StoredBy,
				"storedAt":             entry.StoredAt.UTC().Format(time.RFC3339),
				"contentHash":          entry.ContentHash,
				"snapshotCid":          entry.SnapshotCID,
				"snapshotCidCreatedAt": entry.SnapshotCIDCreatedAt.UTC().Format(time.RFC3339),
				"snapshotPresent":      entry.ContractSnapshot.IsNotNullValue(),
				"signatureStatus":      archiveJSONStatus(entry.SignatureMeta),
				"credentialHashStatus": archiveJSONStatus(entry.CredentialHashes),
				"retentionUntil":       formatOptionalTime(entry.RetentionUntil),
				"deletedAt":            formatOptionalTime(entry.DeletedAt),
				"deletedBy":            entry.DeletedBy,
				"deletionReason":       entry.DeletionReason,
			},
			Did:       &did,
			CreatedAt: entry.StoredAt.UTC().Format(time.RFC3339),
			Kind:      stringPointer("TIMELINE"),
		})
		// Integrity checks cover live entries only: a soft-deleted entry's
		// snapshot was disposed of and its CEK shredded (DCS-NFR-SEC-13), so
		// re-fetching it would report the erasure as permanent corruption.
		// The timeline summary above still documents the deletion itself.
		if entry.DeletedAt != nil {
			continue
		}
		archiveStoreEvents, err := s.ATrailReader.ReadAuditLogEntriesByComponentAndDID(ctx, tx, componenttype.ContractStorageArchive, did)
		if err != nil {
			return nil, err
		}
		integrityEntries := s.archiveIntegrityTrailEntries(ctx, entry, i, archiveStoreEvents, notaryEvents, chainErr)
		result[did] = append(result[did], integrityEntries...)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

// archiveOperationLog returns, per DID, the archive operations recorded in the
// ContractStorageArchive audit chains (DCS-IR-CSA-04, DCS-FR-CSA-18): store,
// retrieve, search, annotate, delete, evidence, sections, failures and key
// erasure. allowed limits the DIDs as readableContractDIDs does; the chain of
// the operations that name no single contract ("*") is kept only for a
// caller without that limit and without a did filter.
func archiveOperationLog(chains [][]datatype.AuditLogEntry, did string, allowed map[string]bool) map[string][]*processauditandcompliance.PACResourceAuditTrailEntry {
	result := map[string][]*processauditandcompliance.PACResourceAuditTrailEntry{}
	for _, chain := range chains {
		for _, entry := range chain {
			if entry.DID == nil || !base.IsAuditVisibleEventType(entry.EventType) {
				continue
			}
			entryDID := *entry.DID
			if (did != "" && entryDID != did) || (allowed != nil && !allowed[entryDID]) {
				continue
			}
			result[entryDID] = append(result[entryDID], &processauditandcompliance.PACResourceAuditTrailEntry{
				ID: entry.ID, Component: entry.Component, EventType: entry.EventType, EventData: entry.EventData,
				Did: entry.DID, CreatedAt: entry.CreatedAt.UTC().Format(time.RFC3339), ResLogPredCid: entry.ResLogPredCID,
				Kind: stringPointer("TIMELINE"),
			})
		}
	}
	return result
}

func stringPointer(value string) *string { return &value }

func archiveJSONStatus(raw *datatype.JSON) string {
	return archiveJSONField(raw, "status")
}

// archiveJSONField returns a string field of an archive evidence document;
// empty when the document or the field is absent.
func archiveJSONField(raw *datatype.JSON, key string) string {
	if raw == nil {
		return ""
	}
	bytes, err := raw.MarshalJSON()
	if err != nil {
		return ""
	}
	var data map[string]any
	if err := json.Unmarshal(bytes, &data); err != nil {
		return ""
	}
	value, _ := data[key].(string)
	return value
}

func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
