package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/identity"
	dcstodcsdb "digital-contracting-service/internal/dcstodcs/db"

	contractstoragearchive "digital-contracting-service/gen/contract_storage_archive"
	contractworkflowengine "digital-contracting-service/gen/contract_workflow_engine"
	"digital-contracting-service/internal/auth"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/contractreadpolicy"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	baseevent "digital-contracting-service/internal/base/event"
	cwecommand "digital-contracting-service/internal/contractworkflowengine/command"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
	"digital-contracting-service/internal/contractworkflowengine/query/contract"
	"digital-contracting-service/internal/middleware"

	"github.com/jmoiron/sqlx"
	"goa.design/clue/log"
)

// contractEraser triggers the erasure of a contract's content-encryption
// keys: local shred plus the peer erase handshake (dcstodcs.ContractEraser).
type contractEraser interface {
	EraseContract(ctx context.Context, contractIRI, actor, reason string) error
}

// ContractStorageArchive service implementation.
type contractStorageArchivesrvc struct {
	DB            *sqlx.DB
	CRepo         db.ContractRepo
	DIDDocument   identity.DIDDocument
	ATrailReader  base.AuditTrailReader
	SnapshotStore archiveSnapshotStore
	Eraser        contractEraser
	Keys          artifactstore.CEKRepository
	ERepo         dcstodcsdb.EraseRepository
	Artifacts     archiveArtifacts
	ArchiveNotary cwecommand.ArchiveNotary
	ArchiveTSA    cwecommand.ArchiveTimestampIssuer
	auth.JWTAuthenticator
}

// NewContractStorageArchive returns the ContractStorageArchive service implementation.
func NewContractStorageArchive(db *sqlx.DB, jwtAuth auth.JWTAuthenticator, cRepo db.ContractRepo, didDocument identity.DIDDocument, auditTrailReader base.AuditTrailReader, snapshotStore archiveSnapshotStore,
	eraser contractEraser, keys artifactstore.CEKRepository, eraseRepo dcstodcsdb.EraseRepository,
	archiveNotary cwecommand.ArchiveNotary, archiveTSA cwecommand.ArchiveTimestampIssuer) contractstoragearchive.Service {
	svc := &contractStorageArchivesrvc{
		JWTAuthenticator: jwtAuth,
		DB:               db,
		CRepo:            cRepo,
		DIDDocument:      didDocument,
		ATrailReader:     auditTrailReader,
		SnapshotStore:    snapshotStore,
		Eraser:           eraser,
		Keys:             keys,
		ERepo:            eraseRepo,
		ArchiveNotary:    archiveNotary,
		ArchiveTSA:       archiveTSA,
	}
	if auditTrailReader.Artifacts != nil {
		svc.Artifacts = auditTrailReader.Artifacts
	}
	return svc
}

func (s *contractStorageArchivesrvc) Retrieve(ctx context.Context, p *contractstoragearchive.ArchiveRetrieveRequest) (res *contractstoragearchive.ArchiveRetrieveResponse, err error) {
	defer func() { s.auditFailure(ctx, "retrieve", "", err) }()
	ctx, err = contractReadContext(ctx, s.DIDDocument)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	qry := contract.GetArchivedContractsQry{
		RetrievedBy: middleware.GetParticipantID(ctx),
		HolderDID:   middleware.GetHolderDID(ctx),
		UserRoles:   middleware.GetUserRoles(ctx),
	}

	queryHandler := contract.GetArchivedContractsHandler{
		DB:    s.DB,
		CRepo: s.CRepo,
	}

	result, err := queryHandler.Handle(ctx, qry)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	var contracts []*contractstoragearchive.ContractItem
	for _, item := range result.Contracts {
		contracts = append(contracts, toArchiveContractItem(item))
	}

	return &contractstoragearchive.ArchiveRetrieveResponse{
		Contracts: contracts,
	}, nil
}

func (s *contractStorageArchivesrvc) Search(ctx context.Context, p *contractstoragearchive.ArchiveSearchRequest) (res []*contractstoragearchive.ContractItem, err error) {
	defer func() { s.auditFailure(ctx, "search", stringValue(p.Did), err) }()
	ctx, err = contractReadContext(ctx, s.DIDDocument)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	var state *contractstate.ContractState
	if p.State != nil {
		tState, err := contractstate.NewContractState(*p.State)
		if err != nil {
			return nil, contractstoragearchive.MakeBadRequest(err)
		}
		state = &tState
	}

	validFrom, err := parseSearchTime("valid_from", p.ValidFrom)
	if err != nil {
		return nil, contractstoragearchive.MakeBadRequest(err)
	}
	validUntil, err := parseSearchTime("valid_until", p.ValidUntil)
	if err != nil {
		return nil, contractstoragearchive.MakeBadRequest(err)
	}

	qry := contract.SearchArchivedContractsQry{
		DID:             stringValue(p.Did),
		ContractVersion: intValue(p.ContractVersion),
		State:           state,
		RetrievedBy:     middleware.GetParticipantID(ctx),
		HolderDID:       middleware.GetHolderDID(ctx),
		UserRoles:       middleware.GetUserRoles(ctx),
		Name:            stringValue(p.Name),
		Description:     stringValue(p.Description),
		ContractData:    stringValue(p.ContractData),
		Tag:             stringValue(p.Tag),
		Party:           stringValue(p.Party),
		ValidFrom:       validFrom,
		ValidUntil:      validUntil,
		Jurisdiction:    stringValue(p.Jurisdiction),
	}
	queryHandler := contract.GetArchivedContractsHandler{
		DB:    s.DB,
		CRepo: s.CRepo,
	}

	result, err := queryHandler.Search(ctx, qry)
	if err != nil {
		return nil, contractworkflowengine.MakeInternalError(err)
	}

	var contracts []*contractstoragearchive.ContractItem
	for _, item := range result.Contracts {
		contracts = append(contracts, toArchiveContractItem(item))
	}

	return contracts, nil
}

// Delete soft-deletes every not-yet-deleted archive entry for the given DID
// (DCS-FR-CSA-17) and erases the contract. The caller must be able to read the
// contract, and the contract must no longer be in force
// (contractstate.Erasable). The row is marked deleted_at/deleted_by/
// deletion_reason, never physically removed, and the operation is logged as an
// audit event under the ContractStorageArchive component. After the commit the
// contract's keys are shredded first: the archived objects are encrypted under
// them, so the shred alone makes them unreadable. The removal of the objects
// from IPFS (DCS-NFR-SEC-13) follows on a best-effort basis. A repeated delete
// of an already deleted entry runs the erasure again, so an erasure that
// failed can be completed.
func (s *contractStorageArchivesrvc) Delete(ctx context.Context, p *contractstoragearchive.DeletePayload) (res int, err error) {
	defer func() { s.auditFailure(ctx, "delete", p.Did, err) }()
	if err := authorizeContractReadDocument(ctx, s.DB, s.DIDDocument, p.Did); err != nil {
		return 0, err
	}

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return 0, contractstoragearchive.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `SELECT 1 FROM contracts WHERE did = $1 FOR UPDATE`, p.Did); err != nil {
		return 0, contractstoragearchive.MakeInternalError(err)
	}
	var state contractstate.ContractState
	if err := tx.GetContext(ctx, &state, `SELECT state FROM contracts_effective WHERE did = $1`, p.Did); err != nil {
		return 0, contractstoragearchive.MakeInternalError(err)
	}
	if !state.Erasable() {
		return 0, contractstoragearchive.MakeBadRequest(fmt.Errorf(
			"contract %q is %s; only a TERMINATED, EXPIRED, REVOKED, REJECTED or WITHDRAWN contract can be deleted from the archive", p.Did, state))
	}

	// The object CIDs must be captured before the soft-delete: afterwards
	// there is no other record of which IPFS objects the entries pointed at.
	entries, err := s.CRepo.ReadArchiveEntries(contractreadpolicy.WithSystemScope(ctx), tx)
	if err != nil {
		return 0, contractstoragearchive.MakeInternalError(err)
	}
	objectCIDs := snapshotCIDsForDeletion(entries, p.Did)
	var evidenceCIDs []string
	if err := tx.SelectContext(ctx, &evidenceCIDs, `SELECT content_cid FROM contract_archive_evidence WHERE did = $1
		UNION SELECT content_cid FROM contract_archive_sections WHERE did = $1`, p.Did); err != nil {
		return 0, contractstoragearchive.MakeInternalError(err)
	}
	objectCIDs = append(objectCIDs, evidenceCIDs...)
	if until := retainedUntil(entries, p.Did, time.Now().UTC()); until != nil {
		return 0, contractstoragearchive.MakeBadRequest(fmt.Errorf(
			"archive entry for DID %q is retained until %s (retention policy)", p.Did, until.Format(time.RFC3339)))
	}

	deletedBy := middleware.GetParticipantID(ctx)
	affected, err := s.CRepo.MarkArchiveEntryDeleted(ctx, tx, p.Did, deletedBy, p.Justification)
	if err != nil {
		return 0, contractstoragearchive.MakeInternalError(err)
	}
	if affected == 0 && !hasArchiveEntry(entries, p.Did) {
		return 0, contractstoragearchive.MakeBadRequest(fmt.Errorf("no archive entry found for DID %q", p.Did))
	}
	if affected > 0 {
		evt := contractevents.DeleteArchivedEvent{
			DID:           p.Did,
			DeletedBy:     deletedBy,
			Justification: p.Justification,
			EntriesMarked: affected,
			OccurredAt:    time.Now().UTC(),
		}
		if err := baseevent.Create(ctx, tx, evt, componenttype.ContractStorageArchive); err != nil {
			return 0, contractstoragearchive.MakeInternalError(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, contractstoragearchive.MakeInternalError(err)
	}

	// Erasure (DCS-NFR-COMP-03): the contract's wrapped CEKs are shredded
	// locally with a KEY_SHREDDED event and via the peer erase handshake on
	// every counterparty instance. The deletion justification is the recorded
	// shred reason. An unreachable peer leaves a pending contract_erasures row
	// for the retry scheduler. A repeated shred changes nothing.
	if err := s.Eraser.EraseContract(ctx, p.Did, deletedBy, p.Justification); err != nil {
		return 0, contractstoragearchive.MakeInternalError(fmt.Errorf("archive entry deleted, erasure failed (repeat the delete to retry): %w", err))
	}

	if err := removeArchivedSnapshots(s.SnapshotStore, objectCIDs); err != nil {
		log.Printf(ctx, "archive delete of %s: IPFS disposal incomplete, the objects stay encrypted under the shredded key: %v", p.Did, err)
	}

	return affected, nil
}

// ErasureStatus reports the contract's key-erasure state (DCS-NFR-COMP-03,
// DCS-NFR-SEC-13): live or shredded locally (with the destruction record's
// timestamp, actor, and reason) plus the per-peer erase-handshake state —
// pending while a contract_erasures row awaits the peer's confirmation,
// confirmed once the peer acknowledged shredding its own CEKs.
func (s *contractStorageArchivesrvc) ErasureStatus(ctx context.Context, p *contractstoragearchive.ErasureStatusPayload) (res *contractstoragearchive.ArchiveErasureStatusResponse, err error) {
	if err := authorizeContractReadDocument(ctx, s.DB, s.DIDDocument, p.Did); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	records, err := s.Keys.List(ctx, artifactstore.ContractScope(p.Did))
	if err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	requests, err := s.ERepo.ListByDID(ctx, p.Did)
	if err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	return erasureStatusResponse(p.Did, records, requests), nil
}

func erasureStatusResponse(did string, records []artifactstore.CEKRecord, requests []dcstodcsdb.EraseRequest) *contractstoragearchive.ArchiveErasureStatusResponse {

	response := &contractstoragearchive.ArchiveErasureStatusResponse{
		Did:         did,
		LocalStatus: "live",
		Peers:       []*contractstoragearchive.ArchiveErasurePeerStatus{},
	}
	// Shredding marks every record of the scope at once, so any shredded
	// record means the scope is erased; the latest one is the destruction
	// record surfaced to the caller.
	for i := range records {
		record := records[i]
		if record.ShreddedAt == nil {
			continue
		}
		response.LocalStatus = "shredded"
		shreddedAt := record.ShreddedAt.UTC().Format(time.RFC3339)
		response.ShreddedAt = &shreddedAt
		response.ShreddedBy = record.ShreddedBy
		response.ShredReason = record.ShredReason
	}

	for _, request := range requests {
		peer := &contractstoragearchive.ArchiveErasurePeerStatus{
			PeerDid:     request.PeerDID,
			Status:      "pending",
			RequestedAt: request.RequestedAt.UTC().Format(time.RFC3339),
			RetryCount:  request.RetryCount,
		}
		switch {
		case request.ConfirmedAt != nil:
			peer.Status = "confirmed"
			confirmedAt := request.ConfirmedAt.UTC().Format(time.RFC3339)
			peer.ConfirmedAt = &confirmedAt
		case request.RefusedAt != nil:
			peer.Status = "refused"
			peer.LastError = request.LastError
		default:
			peer.LastError = request.LastError
		}
		if request.LastTriedAt != nil {
			lastTriedAt := request.LastTriedAt.UTC().Format(time.RFC3339)
			peer.LastTriedAt = &lastTriedAt
		}
		response.Peers = append(response.Peers, peer)
	}

	return response
}

// auditFailure records an archive operation that did not complete
// (DCS-FR-CSA-18). A failure to record it is logged; the operation's own error
// stays the answer to the caller.
func (s *contractStorageArchivesrvc) auditFailure(ctx context.Context, operation, did string, cause error) {
	if cause == nil {
		return
	}
	ctx = context.WithoutCancel(ctx)
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		log.Printf(ctx, "could not record failed archive %s: %v", operation, err)
		return
	}
	defer func() { _ = tx.Rollback() }()
	evt := contractevents.ArchiveOperationFailedEvent{
		DID:        did,
		Operation:  operation,
		Actor:      middleware.GetParticipantID(ctx),
		Outcome:    "FAILURE",
		Reason:     cause.Error(),
		OccurredAt: time.Now().UTC(),
	}
	if err := baseevent.Create(ctx, tx, evt, componenttype.ContractStorageArchive); err != nil {
		log.Printf(ctx, "could not record failed archive %s: %v", operation, err)
		return
	}
	if err := tx.Commit(); err != nil {
		log.Printf(ctx, "could not record failed archive %s: %v", operation, err)
	}
}

// archiveSnapshotStore is the slice of the IPFS client the delete path needs
// for snapshot disposal (DCS-NFR-SEC-13).
type archiveSnapshotStore interface {
	DeleteFile(cid string) error
}

// retainedUntil returns the latest future retention_until of did's live
// archive entries, or nil when the retention policy permits deletion
// (DCS-FR-CSA-17). The archive trigger refuses the deletion as well; this
// check turns that refusal into a client error.
func retainedUntil(entries []db.ContractArchiveEntry, did string, now time.Time) *time.Time {
	var latest *time.Time
	for _, entry := range entries {
		if entry.DID != did || entry.DeletedAt != nil || entry.RetentionUntil == nil || !entry.RetentionUntil.After(now) {
			continue
		}
		if latest == nil || entry.RetentionUntil.After(*latest) {
			latest = entry.RetentionUntil
		}
	}
	return latest
}

// hasArchiveEntry reports whether did has an archive entry, live or deleted.
func hasArchiveEntry(entries []db.ContractArchiveEntry, did string) bool {
	for _, entry := range entries {
		if entry.DID == did {
			return true
		}
	}
	return false
}

// snapshotCIDsForDeletion collects the distinct snapshot CIDs of the given
// DID's archive entries, deleted ones included, so that a repeated delete
// removes what an earlier attempt left. Entries without a stored snapshot CID
// carry nothing in IPFS and are skipped.
func snapshotCIDsForDeletion(entries []db.ContractArchiveEntry, did string) []string {
	seen := map[string]bool{}
	cids := []string{}
	for _, entry := range entries {
		if entry.DID != did || entry.SnapshotCID == "" {
			continue
		}
		if seen[entry.SnapshotCID] {
			continue
		}
		seen[entry.SnapshotCID] = true
		cids = append(cids, entry.SnapshotCID)
	}
	return cids
}

// removeArchivedSnapshots removes every CID from the IPFS store
// (DCS-NFR-SEC-13). It tries each CID and returns the joined removal errors.
func removeArchivedSnapshots(store archiveSnapshotStore, cids []string) error {
	var errs []error
	for _, cid := range cids {
		if err := store.DeleteFile(cid); err != nil {
			errs = append(errs, fmt.Errorf("remove archived object %s from IPFS: %w", cid, err))
		}
	}
	return errors.Join(errs...)
}

// Annotate sets an archived contract's summary and tags (DCS-FR-CSA-11).
// The summary is the caller's when provided; otherwise an existing summary
// is kept, and if none exists yet one is generated from the archived
// contract's own metadata. Tags replace the entry's tag set when provided.
// Only the annotation columns change — the archive entry's snapshot and
// evidence stay immutable (DB-trigger enforced) — and the operation is
// recorded in the archive audit log.
func (s *contractStorageArchivesrvc) Annotate(ctx context.Context, p *contractstoragearchive.AnnotatePayload) (res *contractstoragearchive.ArchiveAnnotationResponse, err error) {
	defer func() { s.auditFailure(ctx, "annotate", p.Did, err) }()
	if err := authorizeContractReadDocument(ctx, s.DB, s.DIDDocument, p.Did); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := s.CRepo.ReadArchivedContractsByFilter(contractreadpolicy.WithSystemScope(ctx), tx, db.SearchValues{DID: p.Did})
	if err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	if len(rows) == 0 {
		return nil, contractstoragearchive.MakeBadRequest(
			fmt.Errorf("no archive entry found for DID %q", p.Did))
	}
	entry := rows[0]

	summary := ""
	switch {
	case p.Summary != nil && strings.TrimSpace(*p.Summary) != "":
		summary = strings.TrimSpace(*p.Summary)
	case entry.ArchiveSummary != nil && *entry.ArchiveSummary != "":
		summary = *entry.ArchiveSummary
	default:
		summary = generateArchiveSummary(entry)
	}

	var tagsJSON *datatype.JSON
	tags := decodeArchiveTags(entry.ArchiveTags)
	if p.Tags != nil {
		tags = p.Tags
		encoded, err := json.Marshal(p.Tags)
		if err != nil {
			return nil, contractstoragearchive.MakeInternalError(err)
		}
		j := datatype.JSON(encoded)
		tagsJSON = &j
	}

	affected, err := s.CRepo.AnnotateArchiveEntry(ctx, tx, p.Did, summary, tagsJSON)
	if err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	if affected == 0 {
		return nil, contractstoragearchive.MakeBadRequest(
			fmt.Errorf("no live archive entry found for DID %q", p.Did))
	}

	evt := contractevents.AnnotateArchivedEvent{
		DID:         p.Did,
		AnnotatedBy: middleware.GetParticipantID(ctx),
		Summary:     summary,
		Tags:        tags,
		OccurredAt:  time.Now().UTC(),
	}
	if err := baseevent.Create(ctx, tx, evt, componenttype.ContractStorageArchive); err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}

	if err := tx.Commit(); err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}

	return &contractstoragearchive.ArchiveAnnotationResponse{
		Did:     p.Did,
		Summary: summary,
		Tags:    tags,
	}, nil
}

// Audit returns, per readable archived contract, the archive operation log
// (DCS-IR-CSA-04, UC-07-03): every event recorded under
// componenttype.ContractStorageArchive, with the entry summaries and the
// integrity checks. The DID-less "*" chain of the component-wide operations
// (retrieve/search) is returned to a caller who may read every contract. A
// did filter applies before the integrity checks run.
func (s *contractStorageArchivesrvc) Audit(ctx context.Context, p *contractstoragearchive.AuditPayload) (res []*contractstoragearchive.PACAuditResponse, err error) {
	ctx, err = contractReadContext(ctx, s.DIDDocument)
	if err != nil {
		return nil, err
	}
	did := strings.TrimSpace(stringValue(p.Did))
	if did != "" {
		if err := authorizeContractReadDocument(ctx, s.DB, s.DIDDocument, did); err != nil {
			return nil, err
		}
	}
	allowed, err := readableContractDIDs(ctx, s.DB)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	core := processAuditAndCompliancesrvc{DB: s.DB, CRepo: s.CRepo, ATrailReader: s.ATrailReader}
	entriesByDID, err := core.auditArchiveTrailEntries(ctx, did)
	if err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	chains, err := s.readArchiveChains(ctx)
	if err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	for chainDID, operations := range archiveOperationLog(chains, did, allowed) {
		entriesByDID[chainDID] = append(operations, entriesByDID[chainDID]...)
	}
	result := make([]*contractstoragearchive.PACAuditResponse, 0, len(entriesByDID))
	for did, entries := range entriesByDID {
		trail := make([]*contractstoragearchive.PACResourceAuditTrailEntry, 0, len(entries))
		for _, entry := range entries {
			trail = append(trail, &contractstoragearchive.PACResourceAuditTrailEntry{ID: entry.ID, Component: entry.Component, EventType: entry.EventType, EventData: entry.EventData, Did: entry.Did, CreatedAt: entry.CreatedAt, ResLogPredCid: entry.ResLogPredCid, Kind: entry.Kind, Result: entry.Result, RuleID: entry.RuleID, Reason: entry.Reason})
		}
		result = append(result, &contractstoragearchive.PACAuditResponse{Did: did, Component: componenttype.ContractStorageArchive.String(), CreatedAt: time.Now().UTC().Format(time.RFC3339), AuditTrail: trail})
	}
	return result, nil
}

// readArchiveChains reads the ContractStorageArchive audit chains.
func (s *contractStorageArchivesrvc) readArchiveChains(ctx context.Context) ([][]datatype.AuditLogEntry, error) {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	return s.ATrailReader.ReadAuditLogEntriesByComponent(ctx, tx, componenttype.ContractStorageArchive)
}

func toArchiveContractItem(item db.ContractMetadata) *contractstoragearchive.ContractItem {
	var startDate *string
	if item.StartDate != nil {
		s := item.StartDate.Format(time.RFC3339)
		startDate = &s
	}

	var expDate *string
	if item.ExpDate != nil {
		s := item.ExpDate.Format(time.RFC3339)
		expDate = &s
	}

	var expPolicy *string
	if item.ExpPolicy != nil {
		s := *item.ExpPolicy
		expPolicy = &s
	}

	return &contractstoragearchive.ContractItem{
		Did:               item.DID,
		ContractVersion:   item.ContractVersion,
		State:             item.State,
		Name:              item.Name,
		Description:       item.Description,
		CreatedBy:         item.CreatedBy,
		CreatedAt:         item.CreatedAt.Format(time.RFC3339),
		UpdatedAt:         item.UpdatedAt.Format(time.RFC3339),
		StartDate:         startDate,
		ExpDate:           expDate,
		ExpPolicy:         expPolicy,
		ExpNoticePeriod:   item.ExpNoticePeriod,
		Responsible:       item.Responsible,
		Evidence:          archiveEvidenceValue(item.Evidence),
		ArchiveSummary:    item.ArchiveSummary,
		ArchiveTags:       decodeArchiveTags(item.ArchiveTags),
		TerminationReason: item.TerminationReason,
		TerminatedAt:      formatOptionalTimePtr(item.TerminatedAt),
		TerminationRole:   item.TerminationRole,
	}
}

func formatOptionalTimePtr(value *time.Time) *string {
	if value == nil {
		return nil
	}
	formatted := value.UTC().Format(time.RFC3339)
	return &formatted
}

// generateArchiveSummary derives the automatic half of DCS-FR-CSA-11 from
// the archived contract's own metadata.
func generateArchiveSummary(m db.ContractMetadata) string {
	name := m.DID
	if m.Name != nil && *m.Name != "" {
		name = *m.Name
	}
	summary := fmt.Sprintf("%s (version %d, %s), created by %s", name, m.ContractVersion, m.State, m.CreatedBy)
	if m.StartDate != nil && m.ExpDate != nil {
		summary += fmt.Sprintf(", term %s to %s",
			m.StartDate.UTC().Format("2006-01-02"), m.ExpDate.UTC().Format("2006-01-02"))
	}
	return summary
}

// decodeArchiveTags unpacks the archive entry's JSONB tag array; a missing
// or malformed column yields nil (no tags).
func decodeArchiveTags(raw *datatype.JSON) []string {
	if raw == nil || !raw.IsNotNullValue() {
		return nil
	}
	var tags []string
	if err := json.Unmarshal(*raw, &tags); err != nil {
		return nil
	}
	return tags
}

// archiveEvidenceValue decodes a ContractMetadata.Evidence blob (populated
// only for archived-contract queries, joined from
// contract_archive_entries.evidence) into a plain any for the API response.
func archiveEvidenceValue(evidence *datatype.JSON) any {
	if evidence == nil || !evidence.IsNotNullValue() {
		return nil
	}
	var value any
	if err := json.Unmarshal(*evidence, &value); err != nil {
		return nil
	}
	return value
}

// parseSearchTime parses an optional RFC3339 search parameter
// (DCS-FR-CSA-10, DCS-FR-CSA-13); an unparsable value is a hard error.
func parseSearchTime(name string, value *string) (*time.Time, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(*value))
	if err != nil {
		return nil, fmt.Errorf("invalid %s value %q: expected an RFC3339 timestamp", name, *value)
	}
	return &t, nil
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func intValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}
