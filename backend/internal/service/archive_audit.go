package service

import (
	"context"
	"encoding/json"
	"time"

	processauditandcompliance "digital-contracting-service/gen/process_audit_and_compliance"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
)

func (s *processAuditAndCompliancesrvc) auditArchiveTrailEntries(ctx context.Context) (map[string][]*processauditandcompliance.PACResourceAuditTrailEntry, error) {
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
	if len(entries) == 0 {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return result, nil
	}

	chainReader, err := newArchiveNotaryChainReaderFromEnv()
	if err != nil {
		return nil, err
	}
	notaryEvents, err := chainReader.Read(ctx)
	if err != nil {
		return nil, err
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
		})
		archiveStoreEvents, err := s.ATrailReader.ReadAuditLogEntriesByComponentAndDID(ctx, tx, componenttype.ContractStorageArchive, did)
		if err != nil {
			return nil, err
		}
		integrityEntry, err := s.archiveIntegrityTrailEntries(ctx, entry, i, archiveStoreEvents, notaryEvents)
		if err != nil {
			return nil, err
		}
		result[did] = append(result[did], integrityEntry)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

func archiveJSONStatus(raw *datatype.JSON) string {
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
	status, _ := data["status"].(string)
	return status
}

func formatOptionalTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
