package negotiationmerging

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/contractworkflowengine/db"
)

// MergeChangeRequests folds every accepted (not merely proposed) change
// request of contractVersion into a single update. Requests are applied in
// read order, field by field, so a later accepted request silently
// overwrites an earlier one touching the same field (last-write-wins, no
// conflict detection).
func MergeChangeRequests(ctx context.Context, tx *sqlx.Tx, cRepo db.ContractRepo, nRepo db.NegotiationRepo, did string, contractVersion int) (*db.ContractUpdateData, error) {
	changeRequests, err := nRepo.ReadAllAcceptedByContractDIDAndVersion(ctx, tx, did, contractVersion)
	if err != nil {
		return nil, err
	}

	contract, err := cRepo.ReadDataByDID(ctx, tx, did)
	if err != nil {
		return nil, err
	}

	var contractData map[string]any
	err = json.Unmarshal(*contract.ContractData, &contractData)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal contract data: %w", err)
	}
	if contractData == nil {
		contractData = map[string]any{}
	}

	updateData := db.ContractUpdateData{
		DID: contract.DID,
	}
	for _, changeRequest := range changeRequests {

		var change ChangeRequest
		if err := json.Unmarshal(*changeRequest.ChangeRequest, &change); err != nil {
			return nil, fmt.Errorf("could not unmarshal change request: %w", err)
		}

		if change.Name != nil {
			updateData.Name = change.Name
		}

		if change.Description != nil {
			updateData.Description = change.Description
		}

		if change.StartDate != nil {
			sDate, err := time.Parse(time.RFC3339, *change.StartDate)
			if err != nil {
				return nil, err
			}
			updateData.StartDate = &sDate
		}

		if change.ExpDate != nil {
			eDate, err := time.Parse(time.RFC3339, *change.ExpDate)
			if err != nil {
				return nil, err
			}
			updateData.ExpDate = &eDate
		}

		if change.ExpNoticePeriod != nil {
			updateData.ExpNoticePeriod = change.ExpNoticePeriod
		}

		if change.ExpPolicy != nil {
			updateData.ExpPolicy = change.ExpPolicy
		}

		if change.ContractData != nil {
			updatedContractData, err := mergeContractDataChange(contractData, *change.ContractData)
			if err != nil {
				return nil, err
			}
			newContractData, err := datatype.NewJSON(updatedContractData)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal contract data: %w", err)
			}
			normalizedContractData, err := validation.NormalizeContractDataForPersistence(&newContractData, contract.DID, true)
			if err != nil {
				return nil, fmt.Errorf("contract data validation failed after merging change requests: %w", err)
			}
			updateData.ContractData = normalizedContractData
			contractData = updatedContractData
		}
	}

	return &updateData, nil
}

func mergeContractDataChange(contractData map[string]any, rawChange json.RawMessage) (map[string]any, error) {
	var changeData map[string]any
	if err := json.Unmarshal(rawChange, &changeData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal contract data change: %w", err)
	}
	if changeData == nil {
		return contractData, nil
	}
	if _, canonical := changeData["dcs:documentStructure"]; !canonical {
		return nil, fmt.Errorf("change request contract data must use the canonical dcs:documentStructure envelope")
	}
	return changeData, nil
}
