package contract

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	events "digital-contracting-service/internal/contractworkflowengine/event"
)

type GetArchivedContractsResult struct {
	Contracts []db.ContractMetadata
}

type GetArchivedContractsHandler struct {
	DB    *sqlx.DB
	CRepo db.ContractRepo
}

type GetArchivedContractsQry struct {
	RetrievedBy string
	HolderDID   string
	UserRoles   userrole.UserRoles
}

type SearchArchivedContractsQry struct {
	RetrievedBy     string
	HolderDID       string
	UserRoles       userrole.UserRoles
	DID             string
	ContractVersion int
	State           *contractstate.ContractState
	Name            string
	Description     string
	ContractData    string
	Tag             string
	Party           string
	ValidFrom       *time.Time
	ValidUntil      *time.Time
	Jurisdiction    string
}

func (h *GetArchivedContractsHandler) Handle(ctx context.Context, query GetArchivedContractsQry) (*GetArchivedContractsResult, error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not create transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	archivedContractsMetadata, err := h.CRepo.ReadArchivedContracts(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("could not read all contracts: %w", err)
	}

	evt := events.RetrieveArchivedEvent{
		RetrievedBy:  query.RetrievedBy,
		HolderDID:    query.HolderDID,
		UserRoles:    query.UserRoles,
		ContractDIDs: archivedContractDIDs(archivedContractsMetadata),
		OccurredAt:   time.Now().UTC(),
	}

	err = event.Create(ctx, tx, evt, componenttype.ContractStorageArchive)
	if err != nil {
		return nil, fmt.Errorf("could not create event: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	return &GetArchivedContractsResult{
		Contracts: archivedContractsMetadata,
	}, nil
}

func (h *GetArchivedContractsHandler) Search(ctx context.Context, query SearchArchivedContractsQry) (*GetArchivedContractsResult, error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not create transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	var states []string
	if query.State != nil {
		states = []string{query.State.String()}
	}

	searchValues := db.SearchValues{
		DID:             query.DID,
		ContractVersion: query.ContractVersion,
		States:          states,
		Name:            query.Name,
		Description:     query.Description,
		ContractData:    query.ContractData,
		Tag:             query.Tag,
		Party:           query.Party,
		ValidFrom:       query.ValidFrom,
		ValidUntil:      query.ValidUntil,
		Jurisdiction:    query.Jurisdiction,
	}

	archivedContractsMetadata, err := h.CRepo.ReadArchivedContractsByFilter(ctx, tx, searchValues)
	if err != nil {
		return nil, fmt.Errorf("could not search archived contracts: %w", err)
	}

	evt := events.SearchArchivedEvent{
		RetrievedBy:  query.RetrievedBy,
		HolderDID:    query.HolderDID,
		UserRoles:    query.UserRoles,
		Criteria:     searchCriteria(searchValues),
		ContractDIDs: archivedContractDIDs(archivedContractsMetadata),
		OccurredAt:   time.Now().UTC(),
	}

	err = event.Create(ctx, tx, evt, componenttype.ContractStorageArchive)
	if err != nil {
		return nil, fmt.Errorf("could not create event: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	return &GetArchivedContractsResult{
		Contracts: archivedContractsMetadata,
	}, nil
}

// archivedContractDIDs lists the contract IDs a retrieval returned, so the
// audit entry names every contract the caller received (DCS-FR-CSA-18).
func archivedContractDIDs(rows []db.ContractMetadata) []string {
	dids := make([]string, 0, len(rows))
	for _, row := range rows {
		dids = append(dids, row.DID)
	}
	return dids
}

// searchCriteria keeps the filters the caller set; empty filters are left out.
func searchCriteria(v db.SearchValues) map[string]string {
	criteria := map[string]string{}
	set := func(key, value string) {
		if value != "" {
			criteria[key] = value
		}
	}
	set("did", v.DID)
	if v.ContractVersion != 0 {
		set("contract_version", strconv.Itoa(v.ContractVersion))
	}
	set("state", strings.Join(v.States, ","))
	set("name", v.Name)
	set("description", v.Description)
	set("contract_data", v.ContractData)
	set("tag", v.Tag)
	set("party", v.Party)
	if v.ValidFrom != nil {
		set("valid_from", v.ValidFrom.UTC().Format(time.RFC3339))
	}
	if v.ValidUntil != nil {
		set("valid_until", v.ValidUntil.UTC().Format(time.RFC3339))
	}
	set("jurisdiction", v.Jurisdiction)
	return criteria
}
