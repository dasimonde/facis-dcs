package qry

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"runtime"
	"time"

	"golang.org/x/sync/errgroup"

	processauditandcompliance2 "digital-contracting-service/internal/processauditandcompliance"

	"digital-contracting-service/internal/processauditandcompliance/datatype/eventtype"

	"digital-contracting-service/internal/contractworkflowengine/db"
	"digital-contracting-service/internal/middleware"

	"digital-contracting-service/internal/base/validation"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
)

type GetContractContentTrailQry struct {
	RetrievedBy string
	HolderDID   string
	UserRoles   userrole.UserRoles
	// DID restricts the trail to one contract. Empty means the whole
	// auditable corpus.
	DID string
}

type ContractContentTrailAuditor struct {
	DB    *sqlx.DB
	CRepo db.ContractRepo
}

func (h *ContractContentTrailAuditor) Handle(ctx context.Context, query GetContractContentTrailQry) (map[string][]datatype.AuditLogEntry, error) {
	contracts, err := h.readAuditableContracts(ctx, query.DID)
	if err != nil {
		return nil, err
	}

	auditedBy := middleware.GetParticipantID(ctx)
	holderDID := middleware.GetHolderDID(ctx)

	entriesByIndex := make([][]datatype.AuditLogEntry, len(contracts))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(runtime.GOMAXPROCS(0))
	for contractIndex, contract := range contracts {
		group.Go(func() error {
			auditMetadata := validation.ContractContentAuditMetadata{
				ContractDID:     contract.DID,
				ContractVersion: fmt.Sprint(contract.ContractVersion),
				AuditedBy:       auditedBy,
				HolderDID:       holderDID,
			}
			findings, err := validation.AuditContractContent(groupCtx, contract.ContractData, nil, auditMetadata)
			if err != nil {
				return fmt.Errorf("could not audit contract: %w", err)
			}

			entries := make([]datatype.AuditLogEntry, 0, len(findings))
			for findingIndex, finding := range findings {
				data, err := json.Marshal(processauditandcompliance2.ContractContentPolicyFindingEventData(finding, auditMetadata))
				if err != nil {
					return err
				}
				entries = append(entries, datatype.AuditLogEntry{
					ID:        int64(-3000000 - (contractIndex * 10000) - findingIndex),
					Component: componenttype.ContractWorkflowEngine.String(),
					EventType: eventtype.ContractContentPolicyAuditFinding.String(),
					EventData: data,
					DID:       &contract.DID,
					CreatedAt: time.Now().UTC(),
				})
			}
			entriesByIndex[contractIndex] = entries
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}

	result := map[string][]datatype.AuditLogEntry{}
	for contractIndex, contract := range contracts {
		result[contract.DID] = entriesByIndex[contractIndex]
	}
	return result, nil
}

// readAuditableContracts loads the contracts whose content the trail covers:
// just did when one is named, the whole corpus otherwise. Revalidating every
// stored contract is the dominant cost of an unfiltered audit, so a
// single-resource request must not pay it.
func (h *ContractContentTrailAuditor) readAuditableContracts(ctx context.Context, did string) ([]db.Contract, error) {
	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	var dids []string
	if did != "" {
		// An audit may name a resource this instance holds no contract for (a
		// template, a peer-side DID). That is an empty content trail, not an
		// error.
		exists, err := h.CRepo.ExistsByDID(ctx, tx, did)
		if err != nil {
			return nil, fmt.Errorf("could not look up contract: %w", err)
		}
		if exists {
			dids = []string{did}
		}
	} else {
		metadata, err := h.CRepo.ReadAllMetaData(ctx, tx, datatype.Pagination{})
		if err != nil {
			return nil, fmt.Errorf("could not read all metadata: %w", err)
		}
		dids = make([]string, 0, len(metadata))
		for _, meta := range metadata {
			dids = append(dids, meta.DID)
		}
	}

	contracts := make([]db.Contract, 0, len(dids))
	for _, contractDID := range dids {
		contract, err := h.CRepo.ReadDataByDID(ctx, tx, contractDID)
		if err != nil {
			return nil, fmt.Errorf("could not read data: %w", err)
		}
		if contract.ContractData == nil || !contract.ContractData.IsNotNullValue() {
			continue
		}
		contracts = append(contracts, *contract)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}
	return contracts, nil
}
