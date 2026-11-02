package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	contractstoragearchive "digital-contracting-service/gen/contract_storage_archive"
	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/contractreadpolicy"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	baseevent "digital-contracting-service/internal/base/event"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
	"digital-contracting-service/internal/middleware"

	"github.com/lib/pq"
)

// storeSection archives one section of the given archived version of a
// contract, the latest when none is given, for the parties assigned to it
// (DCS-FR-CSA-26). Each party must be
// able to read the contract.
func (s *contractStorageArchivesrvc) storeSection(ctx context.Context, p *contractstoragearchive.StorePayload) (*contractstoragearchive.ArchiveStoreResponse, error) {
	sectionID := strings.TrimSpace(stringValue(p.SectionID))
	parties := make([]string, 0, len(p.Parties))
	for _, party := range p.Parties {
		if party = strings.TrimSpace(party); party != "" && !slices.Contains(parties, party) {
			parties = append(parties, party)
		}
	}
	if sectionID == "" || len(parties) == 0 {
		return nil, contractstoragearchive.MakeBadRequest(errors.New("a section needs a section_id and at least one party"))
	}

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()

	var entry struct {
		ContractVersion int            `db:"contract_version"`
		Snapshot        datatype.JSON  `db:"contract_snapshot"`
		Origin          string         `db:"origin"`
		CreatedBy       string         `db:"created_by"`
		Data            *datatype.JSON `db:"contract_data"`
	}
	err = tx.GetContext(ctx, &entry, `
		SELECT a.contract_version, a.contract_snapshot, COALESCE(c.origin, '') AS origin, c.created_by, c.contract_data
		FROM contract_archive_entries a JOIN contracts c ON c.did = a.did
		WHERE a.did = $1 AND a.archive_status <> 'DELETED' AND ($2 = 0 OR a.contract_version = $2)
		ORDER BY a.contract_version DESC LIMIT 1`, p.Did, intValue(p.ContractVersion))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, contractstoragearchive.MakeBadRequest(fmt.Errorf("no live archive entry for DID %q and version %d", p.Did, intValue(p.ContractVersion)))
	}
	if err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	for _, party := range parties {
		if !(contractreadpolicy.Scope{Organization: party}).MayRead(entry.Origin, entry.CreatedBy, entry.Data) {
			return nil, contractstoragearchive.MakeBadRequest(fmt.Errorf("%q is not a party of contract %q", party, p.Did))
		}
	}
	var snapshot struct {
		ContractData any `json:"contract_data"`
	}
	if err := json.Unmarshal(entry.Snapshot, &snapshot); err != nil {
		return nil, contractstoragearchive.MakeInternalError(fmt.Errorf("decode archive snapshot: %w", err))
	}
	section := findSectionBlock(snapshot.ContractData, sectionID)
	if section == nil {
		return nil, contractstoragearchive.MakeBadRequest(fmt.Errorf("version %d of contract %q has no section %q", entry.ContractVersion, p.Did, sectionID))
	}
	content, err := json.Marshal(section)
	if err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	sum := sha256.Sum256(content)
	contentHash := "sha256:" + hex.EncodeToString(sum[:])
	cid, err := s.Artifacts.Put(ctx, artifactstore.ContractScope(p.Did), content)
	if err != nil {
		return nil, contractstoragearchive.MakeInternalError(fmt.Errorf("store section content: %w", err))
	}
	storedBy := middleware.GetParticipantID(ctx)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO contract_archive_sections (did, contract_version, section_id, parties, content_hash, content_cid, stored_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		p.Did, entry.ContractVersion, sectionID, pq.Array(parties), contentHash, cid, storedBy); err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return nil, contractstoragearchive.MakeBadRequest(fmt.Errorf("section %q of version %d is already archived", sectionID, entry.ContractVersion))
		}
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	evt := contractevents.StoreArchiveSectionEvent{
		DID: p.Did, ContractVersion: entry.ContractVersion, SectionID: sectionID, Parties: parties,
		StoredBy: storedBy, ContentHash: contentHash, OccurredAt: time.Now().UTC(),
	}
	if err := baseevent.Create(ctx, tx, evt, componenttype.ContractStorageArchive); err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	version := entry.ContractVersion
	return &contractstoragearchive.ArchiveStoreResponse{Did: p.Did, Kind: "section", ContractVersion: &version, SectionID: &sectionID, ContentHash: contentHash}, nil
}

// findSectionBlock returns the dcs:Section or dcs:Clause node with the given
// @id anywhere in the contract document.
func findSectionBlock(node any, id string) map[string]any {
	switch value := node.(type) {
	case map[string]any:
		if value["@id"] == id && (hasJSONLDType(value, "dcs:Section") || hasJSONLDType(value, "dcs:Clause")) {
			return value
		}
		for _, child := range value {
			if found := findSectionBlock(child, id); found != nil {
				return found
			}
		}
	case []any:
		for _, child := range value {
			if found := findSectionBlock(child, id); found != nil {
				return found
			}
		}
	}
	return nil
}

func hasJSONLDType(node map[string]any, typ string) bool {
	switch value := node["@type"].(type) {
	case string:
		return value == typ
	case []any:
		return slices.Contains(value, any(typ))
	}
	return false
}

type archiveSectionRow struct {
	ID              string         `db:"id"`
	DID             string         `db:"did"`
	ContractVersion int            `db:"contract_version"`
	SectionID       string         `db:"section_id"`
	Parties         pq.StringArray `db:"parties"`
	ContentHash     string         `db:"content_hash"`
	ContentCID      string         `db:"content_cid"`
	StoredBy        string         `db:"stored_by"`
	StoredAt        time.Time      `db:"stored_at"`
}

// Sections returns the archived sections of a contract that the caller's
// organization is assigned to (DCS-FR-CSA-26). The Auditor and the system
// roles read every section. A request for one section the caller is not
// assigned to is refused. Failed and successful reads are recorded in the
// archive audit log (DCS-FR-CSA-18).
func (s *contractStorageArchivesrvc) Sections(ctx context.Context, p *contractstoragearchive.SectionsPayload) (res []*contractstoragearchive.ArchiveSectionItem, err error) {
	defer func() { s.auditFailure(ctx, "sections", p.Did, err) }()

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	if s.Artifacts == nil {
		return nil, contractstoragearchive.MakeInternalError(errors.New("artifact store is not configured"))
	}
	if err := authorizeContractReadDocument(ctx, s.DB, s.DIDDocument, p.Did); err != nil {
		return nil, err
	}
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()

	sectionID := strings.TrimSpace(stringValue(p.SectionID))
	var rows []archiveSectionRow
	if err := tx.SelectContext(ctx, &rows, `
		SELECT s.id, s.did, s.contract_version, s.section_id, s.parties, s.content_hash, s.content_cid, s.stored_by, s.stored_at
		FROM contract_archive_sections s
		JOIN contract_archive_entries a ON a.did = s.did AND a.contract_version = s.contract_version
		WHERE s.did = $1 AND a.archive_status <> 'DELETED' AND ($2 = '' OR s.section_id = $2)
		ORDER BY s.contract_version, s.stored_at, s.id`, p.Did, sectionID); err != nil {
		return nil, contractstoragearchive.MakeInternalError(err)
	}
	if sectionID != "" && len(rows) == 0 {
		return nil, contractstoragearchive.MakeNotFound(fmt.Errorf("no archived section %q for DID %q", sectionID, p.Did))
	}

	scope := contractreadpolicy.Scope{Organization: middleware.GetParticipantID(ctx), Roles: middleware.GetUserRoles(ctx)}
	res = make([]*contractstoragearchive.ArchiveSectionItem, 0, len(rows))
	var returned []string
	for _, row := range rows {
		if !scope.Privileged() && !slices.Contains(row.Parties, scope.Organization) {
			if sectionID != "" {
				return nil, contractstoragearchive.MakeForbidden(fmt.Errorf("section %q is not assigned to %q", sectionID, scope.Organization))
			}
			continue
		}
		item := &contractstoragearchive.ArchiveSectionItem{
			ID: row.ID, Did: row.DID, ContractVersion: row.ContractVersion, SectionID: row.SectionID,
			Parties: row.Parties, ContentHash: row.ContentHash, StoredBy: row.StoredBy, StoredAt: row.StoredAt.UTC().Format(time.RFC3339),
		}
		raw, err := s.Artifacts.Get(ctx, artifactstore.ContractScope(row.DID), row.ContentCID)
		switch {
		case artifactstore.IsShredded(err):
		case err != nil:
			return nil, contractstoragearchive.MakeInternalError(fmt.Errorf("read section %s: %w", row.ID, err))
		default:
			if err := json.Unmarshal(raw, &item.Content); err != nil {
				return nil, contractstoragearchive.MakeInternalError(fmt.Errorf("decode section %s: %w", row.ID, err))
			}
		}
		res = append(res, item)
		returned = append(returned, row.SectionID)
	}
	evt := contractevents.RetrieveArchivedEvent{
		RetrievedBy:  scope.Organization,
		HolderDID:    middleware.GetHolderDID(ctx),
		UserRoles:    middleware.GetUserRoles(ctx),
		ContractDIDs: []string{p.Did},
		SectionIDs:   returned,
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
