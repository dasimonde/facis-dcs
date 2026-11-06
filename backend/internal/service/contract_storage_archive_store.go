package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	contractstoragearchive "digital-contracting-service/gen/contract_storage_archive"
	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype/componenttype"
	baseevent "digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/base/validation"
	cwecommand "digital-contracting-service/internal/contractworkflowengine/command"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
	"digital-contracting-service/internal/middleware"

	"github.com/jmoiron/sqlx"
)

// archiveArtifacts is the encrypting IPFS store: content is encrypted under
// the contract's key, so the erasure of the contract erases it too.
type archiveArtifacts interface {
	Put(ctx context.Context, scope artifactstore.Scope, plaintext []byte) (string, error)
	Get(ctx context.Context, scope artifactstore.Scope, cid string) ([]byte, error)
}

// Store archives a contract or stores an evidence document for an archived
// contract (DCS-IR-CSA-02, DCS-FR-CSA-25).
func (s *contractStorageArchivesrvc) Store(ctx context.Context, p *contractstoragearchive.StorePayload) (res *contractstoragearchive.ArchiveStoreResponse, err error) {
	defer func() { s.auditFailure(ctx, "store", p.Did, err) }()
	if err := authorizeContractReadDocument(ctx, s.DB, s.DIDDocument, p.Did); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	if s.Artifacts == nil {
		return nil, contractstoragearchive.MakeInternalError(errors.New("artifact store is not configured"))
	}
	switch p.Kind {
	case "contract":
		return s.storeContract(ctx, p.Did)
	case "evidence":
		return s.storeEvidence(ctx, p)
	case "section":
		return s.storeSection(ctx, p)
	}
	return nil, contractstoragearchive.MakeBadRequest(fmt.Errorf("unknown kind %q", p.Kind))
}

func (s *contractStorageArchivesrvc) storeContract(ctx context.Context, did string) (*contractstoragearchive.ArchiveStoreResponse, error) {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()

	// The per-contract lock serializes this store with a concurrent store, the
	// signing workflow and a peer receipt of the same contract, so the check
	// for an existing entry below holds until the insert.
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", did); err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	contract, err := s.CRepo.ReadDataByDID(ctx, tx, did)
	if err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	signatures, peerSignatures, err := archiveSigningEvidence(ctx, tx, did)
	if err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	storedBy := middleware.GetParticipantID(ctx)
	history, err := s.CRepo.ReadHistoryByDID(ctx, tx, did)
	if err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	entry, err := cwecommand.BuildManualArchiveEntry(contract, history, storedBy, signatures, peerSignatures)
	if err != nil {
		return nil, contractstoragearchive.MakeBadRequest(err)
	}
	var archived bool
	if err := tx.GetContext(ctx, &archived,
		`SELECT EXISTS (SELECT 1 FROM contract_archive_entries WHERE did = $1 AND contract_version = $2)`,
		did, entry.ContractVersion); err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	if archived {
		return nil, contractstoragearchive.MakeBadRequest(
			fmt.Errorf("version %d of contract %q is already archived", entry.ContractVersion, did))
	}

	entry.SnapshotCID, err = s.Artifacts.Put(ctx, artifactstore.ContractScope(did), entry.ContractSnapshot)
	if err != nil {
		return nil, contractstoragearchive.MakeInternalError(fmt.Errorf("store archive snapshot: %w", err))
	}
	notaryReceipt, tsaReceipt, err := cwecommand.NotarizeArchiveEntry(ctx, s.ArchiveNotary, s.ArchiveTSA, &entry)
	if err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	if err := s.CRepo.StoreArchiveEntry(ctx, tx, entry); err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	evt := contractevents.StoreArchivedEvent{
		DID:             did,
		ContractVersion: entry.ContractVersion,
		StoredBy:        storedBy,
		ContentHash:     entry.ContentHash,
		SnapshotCID:     entry.SnapshotCID,
		ArchiveStatus:   "STORED",
		NotaryReceipt:   notaryReceipt,
		TSAReceipt:      tsaReceipt,
		EvidenceSummary: contractevents.ArchiveEvidenceSummary{
			SnapshotHashAlgorithm: "SHA-256",
			SignatureStatus:       "SIGNED",
			CredentialHashStatus:  "HASHED",
		},
		OccurredAt: time.Now().UTC(),
	}
	if err := baseevent.Create(ctx, tx, evt, componenttype.ContractStorageArchive); err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	version := entry.ContractVersion
	return &contractstoragearchive.ArchiveStoreResponse{Did: did, Kind: "contract", ContractVersion: &version, ContentHash: entry.ContentHash}, nil
}

// archiveSigningEvidence reads the signatures this instance applied to the
// contract, with the hashes of the credentials of their ceremonies, and the
// signatures a peer applied, as the signing workflow archives them.
func archiveSigningEvidence(ctx context.Context, tx *sqlx.Tx, did string) ([]cwecommand.ArchiveSigningEvidence, json.RawMessage, error) {
	var rows []struct {
		Signer         string     `db:"signer_did"`
		CredentialType string     `db:"credential_type"`
		Field          string     `db:"field_name"`
		SignedAt       *time.Time `db:"signed_at"`
		PDFCID         string     `db:"ipfs_cid"`
		PDFHash        string     `db:"pdf_hash"`
		CeremonyID     string     `db:"ceremony_id"`
		VPToken        string     `db:"vp_token"`
		KbSdHash       string     `db:"kb_sd_hash"`
		PoAVPToken     string     `db:"poa_vp_token"`
	}
	if err := tx.SelectContext(ctx, &rows, `
		SELECT s.signer_did, s.credential_type, COALESCE(s.field_name, '') AS field_name, s.signed_at,
		       COALESCE(s.ipfs_cid, '') AS ipfs_cid, COALESCE(s.pdf_hash, '') AS pdf_hash, COALESCE(c.id::text, '') AS ceremony_id,
		       COALESCE(c.vp_token, '') AS vp_token, COALESCE(c.kb_sd_hash, '') AS kb_sd_hash, COALESCE(c.poa_vp_token, '') AS poa_vp_token
		FROM contract_signatures s LEFT JOIN signature_ceremonies c ON c.id = s.ceremony_id
		WHERE s.contract_did = $1 AND s.status = 'SIGNED'
		ORDER BY s.created_at, s.id`, did); err != nil {
		return nil, nil, fmt.Errorf("read signatures: %w", err)
	}
	signatures := make([]cwecommand.ArchiveSigningEvidence, 0, len(rows))
	for _, row := range rows {
		signing := cwecommand.ArchiveSigningEvidence{
			Signer: row.Signer, CredentialType: row.CredentialType, CeremonyID: row.CeremonyID, Field: row.Field,
			PDFCID: row.PDFCID, PDFHash: row.PDFHash,
			CredentialHashes: cwecommand.ArchiveCredentialHashes(row.VPToken, row.KbSdHash, row.PoAVPToken),
		}
		if row.SignedAt != nil {
			signing.SignedAt = *row.SignedAt
		}
		signatures = append(signatures, signing)
	}
	var peers []byte
	err := tx.GetContext(ctx, &peers, `SELECT poa_evidence FROM contract_sync_signatures WHERE did = $1 AND poa_evidence IS NOT NULL`, did)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, nil, fmt.Errorf("read peer signatures: %w", err)
	}
	return signatures, peers, nil
}

func (s *contractStorageArchivesrvc) storeEvidence(ctx context.Context, p *contractstoragearchive.StorePayload) (*contractstoragearchive.ArchiveStoreResponse, error) {
	name, mediaType := strings.TrimSpace(stringValue(p.Name)), strings.TrimSpace(stringValue(p.MediaType))
	if name == "" || mediaType == "" || len(p.Content) == 0 {
		return nil, contractstoragearchive.MakeBadRequest(errors.New("evidence needs a name, a media type and content"))
	}

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()

	var archived bool
	if err := tx.GetContext(ctx, &archived,
		`SELECT EXISTS (SELECT 1 FROM contract_archive_entries WHERE did = $1 AND archive_status <> 'DELETED')`, p.Did); err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	if !archived {
		return nil, contractstoragearchive.MakeBadRequest(fmt.Errorf("no live archive entry for DID %q", p.Did))
	}

	sum := sha256.Sum256(p.Content)
	contentHash := "sha256:" + hex.EncodeToString(sum[:])
	cid, err := s.Artifacts.Put(ctx, artifactstore.ContractScope(p.Did), p.Content)
	if err != nil {
		return nil, contractstoragearchive.MakeInternalError(fmt.Errorf("store evidence content: %w", err))
	}
	storedBy := middleware.GetParticipantID(ctx)
	var id string
	if err := tx.GetContext(ctx, &id, `
		INSERT INTO contract_archive_evidence (did, name, media_type, description, content_hash, content_cid, size_bytes, stored_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
		p.Did, name, mediaType, p.Description, contentHash, cid, len(p.Content), storedBy); err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	evt := contractevents.StoreArchiveEvidenceEvent{
		DID:         p.Did,
		EvidenceID:  id,
		StoredBy:    storedBy,
		Name:        name,
		MediaType:   mediaType,
		ContentHash: contentHash,
		OccurredAt:  time.Now().UTC(),
	}
	if err := baseevent.Create(ctx, tx, evt, componenttype.ContractStorageArchive); err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	return &contractstoragearchive.ArchiveStoreResponse{Did: p.Did, Kind: "evidence", EvidenceID: &id, ContentHash: contentHash}, nil
}

type archiveEvidenceRow struct {
	ID          string    `db:"id"`
	DID         string    `db:"did"`
	Name        string    `db:"name"`
	MediaType   string    `db:"media_type"`
	Description *string   `db:"description"`
	ContentHash string    `db:"content_hash"`
	ContentCID  string    `db:"content_cid"`
	SizeBytes   int64     `db:"size_bytes"`
	StoredBy    string    `db:"stored_by"`
	StoredAt    time.Time `db:"stored_at"`
}

// Evidence lists the evidence documents of an archived contract with their
// decrypted content. Content of an erased contract is left out. The read is
// recorded in the archive audit log (DCS-FR-CSA-18).
func (s *contractStorageArchivesrvc) Evidence(ctx context.Context, p *contractstoragearchive.EvidencePayload) (res []*contractstoragearchive.ArchiveEvidenceItem, err error) {
	defer func() { s.auditFailure(ctx, "evidence", p.Did, err) }()

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	if s.Artifacts == nil {
		return nil, contractstoragearchive.MakeInternalError(errors.New("artifact store is not configured"))
	}
	localPeer, err := s.DIDDocument.GetID()
	if err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	if err := authorizeContractRead(ctx, s.DB, localPeer, p.Did); err != nil {
		return nil, err
	}
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()

	var rows []archiveEvidenceRow
	if err := tx.SelectContext(ctx, &rows, `
		SELECT id, did, name, media_type, description, content_hash, content_cid, size_bytes, stored_by, stored_at
		FROM contract_archive_evidence WHERE did = $1 ORDER BY stored_at, id`, p.Did); err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	res = make([]*contractstoragearchive.ArchiveEvidenceItem, 0, len(rows))
	for _, row := range rows {
		content, err := s.Artifacts.Get(ctx, artifactstore.ContractScope(row.DID), row.ContentCID)
		if err != nil && !artifactstore.IsShredded(err) {
			return nil, contractstoragearchive.MakeInternalError(fmt.Errorf("read evidence %s: %w", row.ID, err))
		}
		res = append(res, &contractstoragearchive.ArchiveEvidenceItem{
			ID: row.ID, Did: row.DID, Name: row.Name, MediaType: row.MediaType, Description: row.Description,
			ContentHash: row.ContentHash, SizeBytes: row.SizeBytes, StoredBy: row.StoredBy,
			StoredAt: row.StoredAt.UTC().Format(time.RFC3339), Content: content,
		})
	}
	evt := contractevents.RetrieveArchivedEvent{
		RetrievedBy:  middleware.GetParticipantID(ctx),
		HolderDID:    middleware.GetHolderDID(ctx),
		UserRoles:    middleware.GetUserRoles(ctx),
		ContractDIDs: []string{p.Did},
		OccurredAt:   time.Now().UTC(),
	}
	if err := baseevent.Create(ctx, tx, evt, componenttype.ContractStorageArchive); err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	return res, nil
}

// Compliance verifies every live archive entry against the archive
// compliance rules and flags the entries that fail one (DCS-FR-CSA-19).
func (s *contractStorageArchivesrvc) Compliance(ctx context.Context, _ *contractstoragearchive.CompliancePayload) ([]*contractstoragearchive.ArchiveComplianceItem, error) {
	ctx, err := contractReadContext(ctx, s.DIDDocument)
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
	contracts, err := s.CRepo.ReadArchivedContracts(ctx, tx)
	if err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	metadata := make(map[string]db.ContractMetadata, len(contracts))
	for _, contract := range contracts {
		metadata[contract.DID] = contract
	}

	now := time.Now().UTC()
	res := []*contractstoragearchive.ArchiveComplianceItem{}
	for _, entry := range entries {
		if entry.DeletedAt != nil {
			continue
		}
		var meta *db.ContractMetadata
		if m, ok := metadata[entry.DID]; ok {
			meta = &m
		}
		item := &contractstoragearchive.ArchiveComplianceItem{
			Did:             entry.DID,
			ContractVersion: entry.ContractVersion,
			Checks:          archiveComplianceChecks(entry, meta, now),
		}
		if meta != nil {
			item.Name = meta.Name
		}
		for _, check := range item.Checks {
			if !check.Passed {
				item.Flagged = true
			}
		}
		res = append(res, item)
	}
	return res, nil
}

// archiveComplianceChecks evaluates one archive entry against the rules of
// DCS-FR-CSA-19. meta is the contract's metadata; nil when it is unavailable.
func archiveComplianceChecks(entry db.ContractArchiveEntry, meta *db.ContractMetadata, now time.Time) []*contractstoragearchive.ArchiveComplianceCheck {
	check := func(rule string, problems []string) *contractstoragearchive.ArchiveComplianceCheck {
		c := &contractstoragearchive.ArchiveComplianceCheck{Rule: rule, Passed: len(problems) == 0}
		if !c.Passed {
			reason := strings.Join(problems, "; ")
			c.Reason = &reason
		}
		return c
	}

	var retention []string
	if entry.RetentionUntil != nil && now.After(*entry.RetentionUntil) {
		retention = append(retention, "retention period ended on "+entry.RetentionUntil.UTC().Format("2006-01-02")+"; review the entry for deletion")
	}

	signature := archiveSignatureProblems(entry)
	if !archiveEvidencePresent(entry.CredentialHashes) {
		signature = append(signature, "no signer credential hashes recorded")
	}

	metadata := []string{"contract metadata unavailable"}
	if meta != nil {
		var missing []string
		if meta.Name == nil || strings.TrimSpace(*meta.Name) == "" {
			missing = append(missing, "name")
		}
		if meta.Responsible == nil || meta.Responsible.Creator == "" || meta.Responsible.Counterparty == "" {
			missing = append(missing, "parties")
		}
		if meta.StartDate == nil || meta.ExpDate == nil {
			missing = append(missing, "validity period")
		}
		metadata = nil
		if len(missing) > 0 {
			metadata = []string{"missing " + strings.Join(missing, ", ")}
		}
	}

	var evidence []string
	if !entry.ContractSnapshot.IsNotNullValue() || entry.ContentHash == "" {
		evidence = append(evidence, "no contract snapshot")
	}
	if !archiveEvidencePresent(entry.TSAReceipt) {
		evidence = append(evidence, "no TSA receipt")
	}

	return []*contractstoragearchive.ArchiveComplianceCheck{
		check("retention", retention),
		check("signature_policy", signature),
		check("metadata_completeness", metadata),
		check("evidence_completeness", evidence),
	}
}

// archiveSignatureProblems judges the signatures an archive entry records
// (cwecommand.BuildArchiveEntry): each signature this instance applied must
// name its credential type, and that level must meet the level the archived
// contract requires for its field. A signature a counterparty applied on its
// own instance must name its party and signatory; its level is judged on that
// instance (ADR-37).
func archiveSignatureProblems(entry db.ContractArchiveEntry) []string {
	var metadata struct {
		Status     string `json:"status"`
		Signatures []struct {
			Signer         string `json:"signer"`
			Field          string `json:"field"`
			CredentialType string `json:"credential_type"`
		} `json:"signatures"`
		PeerSignatures []struct {
			Party     string `json:"party"`
			Signatory string `json:"signatory"`
		} `json:"peer_signatures"`
	}
	if entry.SignatureMeta != nil {
		_ = json.Unmarshal(*entry.SignatureMeta, &metadata)
	}
	if metadata.Status != "SIGNED" || len(metadata.Signatures)+len(metadata.PeerSignatures) == 0 {
		return []string{"no completed signature recorded"}
	}
	var snapshot struct {
		ContractData json.RawMessage `json:"contract_data"`
	}
	_ = json.Unmarshal(entry.ContractSnapshot, &snapshot)
	var problems []string
	for _, signature := range metadata.Signatures {
		required := validation.RequiredCredentialType(snapshot.ContractData, signature.Field)
		switch {
		case signature.CredentialType == "":
			problems = append(problems, fmt.Sprintf("signature of %s for %q records no credential type", signature.Signer, signature.Field))
		case !validation.SignatureLevelSatisfies(signature.CredentialType, required):
			problems = append(problems, fmt.Sprintf("signature of %s for %q reached %s, the contract requires %s", signature.Signer, signature.Field, signature.CredentialType, required))
		}
	}
	for _, signature := range metadata.PeerSignatures {
		if signature.Party == "" || signature.Signatory == "" {
			problems = append(problems, "a counterparty signature records no party or signatory")
		}
	}
	return problems
}
