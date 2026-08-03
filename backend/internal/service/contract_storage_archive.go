package service

import (
	"context"
	"encoding/json"
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
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	baseevent "digital-contracting-service/internal/base/event"
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
	auth.JWTAuthenticator
}

// NewContractStorageArchive returns the ContractStorageArchive service implementation.
func NewContractStorageArchive(db *sqlx.DB, jwtAuth auth.JWTAuthenticator, cRepo db.ContractRepo, didDocument identity.DIDDocument, auditTrailReader base.AuditTrailReader, snapshotStore archiveSnapshotStore,
	eraser contractEraser, keys artifactstore.CEKRepository, eraseRepo dcstodcsdb.EraseRepository) contractstoragearchive.Service {
	return &contractStorageArchivesrvc{
		JWTAuthenticator: jwtAuth,
		DB:               db,
		CRepo:            cRepo,
		DIDDocument:      didDocument,
		ATrailReader:     auditTrailReader,
		SnapshotStore:    snapshotStore,
		Eraser:           eraser,
		Keys:             keys,
		ERepo:            eraseRepo,
	}
}

func (s *contractStorageArchivesrvc) Retrieve(ctx context.Context, p *contractstoragearchive.ArchiveRetrieveRequest) (res *contractstoragearchive.ArchiveRetrieveResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	qry := contract.GetArchivedContractsQry{
		RetrievedBy: middleware.GetParticipantID(ctx),
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

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	var state *contractstate.ContractState
	if p.State != nil {
		tState, err := contractstate.NewContractState(*p.State)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
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
		Name:            stringValue(p.Name),
		Description:     stringValue(p.Description),
		ContractData:    stringValue(p.ContractData),
		Tag:             stringValue(p.Tag),
		Party:           stringValue(p.Party),
		ValidFrom:       validFrom,
		ValidUntil:      validUntil,
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

func (s *contractStorageArchivesrvc) Store(ctx context.Context, p *contractstoragearchive.StorePayload) (res string, err error) {
	log.Printf(ctx, "contractStorageArchive.store")
	return
}

// Delete soft-deletes every not-yet-deleted archive entry for the given DID
// (DCS-FR-CSA-17): the row is marked deleted_at/deleted_by/deletion_reason,
// never physically removed, and the operation is itself logged as an audit
// event under the ContractStorageArchive component so it shows up through
// Audit below. Once the soft-delete is committed, the entries' archived
// snapshots are removed from the IPFS store — secure data disposal per
// DCS-NFR-SEC-13 — and a removal failure is a hard error.
func (s *contractStorageArchivesrvc) Delete(ctx context.Context, p *contractstoragearchive.DeletePayload) (res int, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return 0, contractstoragearchive.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()

	// The snapshot CIDs must be captured before the soft-delete: afterwards
	// there is no other record of which IPFS objects the entries pointed at
	// for this deletion.
	entries, err := s.CRepo.ReadArchiveEntries(ctx, tx)
	if err != nil {
		return 0, contractstoragearchive.MakeInternalError(err)
	}
	snapshotCIDs := snapshotCIDsForDeletion(entries, p.Did)

	deletedBy := middleware.GetParticipantID(ctx)
	affected, err := s.CRepo.MarkArchiveEntryDeleted(ctx, tx, p.Did, deletedBy, p.Justification)
	if err != nil {
		return 0, contractstoragearchive.MakeInternalError(err)
	}
	if affected == 0 {
		return 0, contractstoragearchive.MakeBadRequest(
			fmt.Errorf("no archive entry found for DID %q (or it was already deleted)", p.Did))
	}

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

	if err := tx.Commit(); err != nil {
		return 0, contractstoragearchive.MakeInternalError(err)
	}

	// Secure disposal (DCS-NFR-SEC-13): the archived snapshots leave the
	// IPFS store now that their entries are deleted.
	if err := removeArchivedSnapshots(s.SnapshotStore, snapshotCIDs); err != nil {
		return 0, contractstoragearchive.MakeInternalError(err)
	}

	// Erasure (DCS-NFR-COMP-03): with the archive entries soft-deleted (so
	// the integrity audit no longer covers them), the contract's wrapped CEKs
	// are shredded — locally with a KEY_SHREDDED event, and via the peer
	// erase handshake on every counterparty instance. The deletion
	// justification is the recorded shred reason. An unreachable peer leaves
	// a pending contract_erasures row for the retry scheduler.
	if err := s.Eraser.EraseContract(ctx, p.Did, deletedBy, p.Justification); err != nil {
		return 0, contractstoragearchive.MakeInternalError(err)
	}

	return affected, nil
}

// ErasureStatus reports the contract's key-erasure state (DCS-NFR-COMP-03,
// DCS-NFR-SEC-13): live or shredded locally (with the destruction record's
// timestamp, actor, and reason) plus the per-peer erase-handshake state —
// pending while a contract_erasures row awaits the peer's confirmation,
// confirmed once the peer acknowledged shredding its own CEKs.
func (s *contractStorageArchivesrvc) ErasureStatus(ctx context.Context, p *contractstoragearchive.ErasureStatusPayload) (res *contractstoragearchive.ArchiveErasureStatusResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	records, err := s.Keys.List(ctx, artifactstore.ContractScope(p.Did))
	if err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	response := &contractstoragearchive.ArchiveErasureStatusResponse{
		Did:         p.Did,
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

	requests, err := s.ERepo.ListByDID(ctx, p.Did)
	if err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	for _, request := range requests {
		peer := &contractstoragearchive.ArchiveErasurePeerStatus{
			PeerDid:     request.PeerDID,
			Status:      "pending",
			RequestedAt: request.RequestedAt.UTC().Format(time.RFC3339),
			RetryCount:  request.RetryCount,
		}
		if request.ConfirmedAt != nil {
			peer.Status = "confirmed"
			confirmedAt := request.ConfirmedAt.UTC().Format(time.RFC3339)
			peer.ConfirmedAt = &confirmedAt
		}
		if request.LastTriedAt != nil {
			lastTriedAt := request.LastTriedAt.UTC().Format(time.RFC3339)
			peer.LastTriedAt = &lastTriedAt
		}
		response.Peers = append(response.Peers, peer)
	}

	return response, nil
}

// archiveSnapshotStore is the slice of the IPFS client the delete path needs
// for snapshot disposal (DCS-NFR-SEC-13).
type archiveSnapshotStore interface {
	DeleteFile(cid string) error
}

// snapshotCIDsForDeletion collects the distinct snapshot CIDs of the given
// DID's live (not yet soft-deleted) archive entries — the IPFS objects a
// delete of that DID must dispose of. Entries without a stored snapshot CID
// carry nothing in IPFS and are skipped.
func snapshotCIDsForDeletion(entries []db.ContractArchiveEntry, did string) []string {
	seen := map[string]bool{}
	cids := []string{}
	for _, entry := range entries {
		if entry.DID != did || entry.DeletedAt != nil || entry.SnapshotCID == "" {
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

// removeArchivedSnapshots removes each snapshot CID from the IPFS store,
// failing hard on the first removal error (DCS-NFR-SEC-13).
func removeArchivedSnapshots(store archiveSnapshotStore, cids []string) error {
	for _, cid := range cids {
		if err := store.DeleteFile(cid); err != nil {
			return fmt.Errorf("remove archived snapshot %s from IPFS: %w", cid, err)
		}
	}
	return nil
}

// Annotate sets an archived contract's summary and tags (DCS-FR-CSA-11).
// The summary is the caller's when provided; otherwise an existing summary
// is kept, and if none exists yet one is generated from the archived
// contract's own metadata. Tags replace the entry's tag set when provided.
// Only the annotation columns change — the archive entry's snapshot and
// evidence stay immutable (DB-trigger enforced) — and the operation is
// recorded in the archive audit log.
func (s *contractStorageArchivesrvc) Annotate(ctx context.Context, p *contractstoragearchive.AnnotatePayload) (res *contractstoragearchive.ArchiveAnnotationResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := s.CRepo.ReadArchivedContractsByFilter(ctx, tx, db.SearchValues{DID: p.Did})
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

// Audit returns the archive component's audit log (DCS-IR-CSA-04,
// UC-07-03): every event recorded under componenttype.ContractStorageArchive
// (store/retrieve/search/delete), across every DID's chain plus the
// DID-less "*" chain used by component-wide operations (retrieve/search) —
// reusing the same cross-component reader process_audit_and_compliance's
// own audit method is built on (qry.Auditor / ReadAuditLogEntriesByComponent).
func (s *contractStorageArchivesrvc) Audit(ctx context.Context, p *contractstoragearchive.AuditPayload) (res []*contractstoragearchive.PACAuditResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	core := processAuditAndCompliancesrvc{DB: s.DB, CRepo: s.CRepo, ATrailReader: s.ATrailReader}
	entriesByDID, err := core.auditArchiveTrailEntries(ctx)
	if err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	result := make([]*contractstoragearchive.PACAuditResponse, 0, len(entriesByDID))
	for did, entries := range entriesByDID {
		if p.Did != nil && strings.TrimSpace(*p.Did) != "" && did != strings.TrimSpace(*p.Did) {
			continue
		}
		trail := make([]*contractstoragearchive.PACResourceAuditTrailEntry, 0, len(entries))
		for _, entry := range entries {
			trail = append(trail, &contractstoragearchive.PACResourceAuditTrailEntry{ID: entry.ID, Component: entry.Component, EventType: entry.EventType, EventData: entry.EventData, Did: entry.Did, CreatedAt: entry.CreatedAt, ResLogPredCid: entry.ResLogPredCid, Kind: entry.Kind, Result: entry.Result, RuleID: entry.RuleID, Reason: entry.Reason})
		}
		result = append(result, &contractstoragearchive.PACAuditResponse{Did: did, Component: componenttype.ContractStorageArchive.String(), CreatedAt: time.Now().UTC().Format(time.RFC3339), AuditTrail: trail})
	}
	return result, nil
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
		Did:             item.DID,
		ContractVersion: item.ContractVersion,
		State:           item.State,
		Name:            item.Name,
		Description:     item.Description,
		CreatedBy:       item.CreatedBy,
		CreatedAt:       item.CreatedAt.Format(time.RFC3339),
		UpdatedAt:       item.UpdatedAt.Format(time.RFC3339),
		StartDate:       startDate,
		ExpDate:         expDate,
		ExpPolicy:       expPolicy,
		ExpNoticePeriod: item.ExpNoticePeriod,
		Responsible:     item.Responsible,
		Evidence:        archiveEvidenceValue(item.Evidence),
		ArchiveSummary:  item.ArchiveSummary,
		ArchiveTags:     decodeArchiveTags(item.ArchiveTags),
	}
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
