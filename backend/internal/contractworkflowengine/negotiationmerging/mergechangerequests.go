package negotiationmerging

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/contractworkflowengine/db"
)

// MergeChangeRequests folds every accepted (not merely proposed) change
// request of contractVersion into a single update. Requests are applied
// oldest first, field by field, so a later accepted request overwrites an
// earlier one touching the same field (last-accepted-wins, no conflict
// detection, no field-level union).
//
// The fold therefore has no meaning without a total order over the accepted
// set. It is established here rather than trusted from the repository,
// because the result is bound by digest into the settlement artifact
// (signingmanagement/command.assertCounterpartiesSettled) and a document that
// depends on which row a query happened to return first is a document the
// counterparty cannot be held to.
//
// The second return value names every accepted request an overwrite dropped
// content from, and the request that beat it. Accepting a change request and
// having its content reach the contract are two different things under
// last-accepted-wins, and the decision row records only the first; the caller
// is expected to persist and audit these so the record cannot claim a party
// agreed to wording the contract never carried (DCS-IR-CWE-03).
func MergeChangeRequests(ctx context.Context, tx *sqlx.Tx, cRepo db.ContractRepo, nRepo db.NegotiationRepo, did string, contractVersion int) (*db.ContractUpdateData, []db.NegotiationSupersession, error) {
	changeRequests, err := nRepo.ReadAllAcceptedByContractDIDAndVersion(ctx, tx, did, contractVersion)
	if err != nil {
		return nil, nil, err
	}
	slices.SortStableFunc(changeRequests, func(a, b db.NegotiationChangeData) int {
		if applied := a.CreatedAt.Compare(b.CreatedAt); applied != 0 {
			return applied
		}
		return strings.Compare(a.ID, b.ID)
	})

	contract, err := cRepo.ReadDataByDID(ctx, tx, did)
	if err != nil {
		return nil, nil, err
	}

	var contractData map[string]any
	err = json.Unmarshal(*contract.ContractData, &contractData)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal contract data: %w", err)
	}
	if contractData == nil {
		contractData = map[string]any{}
	}

	discarded := newDiscardLog()
	updateData := db.ContractUpdateData{
		DID: contract.DID,
	}
	for _, changeRequest := range changeRequests {

		var change ChangeRequest
		if err := json.Unmarshal(*changeRequest.ChangeRequest, &change); err != nil {
			return nil, nil, fmt.Errorf("could not unmarshal change request: %w", err)
		}

		discarded.claim(changeRequest.ID, fieldsSetBy(change))

		if change.Name != nil {
			updateData.Name = change.Name
		}

		if change.Description != nil {
			updateData.Description = change.Description
		}

		if change.StartDate != nil {
			sDate, err := time.Parse(time.RFC3339, *change.StartDate)
			if err != nil {
				return nil, nil, err
			}
			updateData.StartDate = &sDate
		}

		if change.ExpDate != nil {
			eDate, err := time.Parse(time.RFC3339, *change.ExpDate)
			if err != nil {
				return nil, nil, err
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
				return nil, nil, err
			}
			newContractData, err := datatype.NewJSON(updatedContractData)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to marshal contract data: %w", err)
			}
			normalizedContractData, err := validation.NormalizeContractDataForPersistence(&newContractData, contract.DID, true)
			if err != nil {
				return nil, nil, fmt.Errorf("contract data validation failed after merging change requests: %w", err)
			}
			updateData.ContractData = normalizedContractData
			contractData = updatedContractData
		}
	}

	return &updateData, discarded.records(), nil
}

// mergeContractDataChange resolves the round's document: last-accepted-wins is
// whole-document, so an accepted request's document replaces the one the round
// started from rather than being blended into it. Every earlier accepted
// request's document is dropped entire, which is why the caller must be told
// (see discardLog) — a field-level union would instead produce a document no
// party ever reviewed and then bind it into the settlement digest.
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

// fieldsSetBy names the parts of the contract this change request asks to
// change, using the request's own JSON keys so the discard record reads back
// as what the negotiator submitted. A key the request left unset cannot be
// discarded, because the fold never applies it.
func fieldsSetBy(change ChangeRequest) []string {
	var fields []string
	if change.Name != nil {
		fields = append(fields, "name")
	}
	if change.Description != nil {
		fields = append(fields, "description")
	}
	if change.StartDate != nil {
		fields = append(fields, "start_date")
	}
	if change.ExpDate != nil {
		fields = append(fields, "exp_date")
	}
	if change.ExpNoticePeriod != nil {
		fields = append(fields, "exp_notice_period")
	}
	if change.ExpPolicy != nil {
		fields = append(fields, "exp_policy")
	}
	if change.ContractData != nil {
		fields = append(fields, "contract_data")
	}
	return fields
}

// discardLog watches the fold happen and notes each overwrite: which accepted
// request last set a field, and therefore which earlier accepted request lost
// it. Observation only — it does not alter what the fold produces.
type discardLog struct {
	holder map[string]string
	// entries is keyed by loser+winner so one pair accumulates its fields into
	// a single record; order preserves the sequence in which the overwrites
	// happened, which is the proposal order of the requests.
	entries map[[2]string]*db.NegotiationSupersession
	order   [][2]string
}

func newDiscardLog() *discardLog {
	return &discardLog{holder: map[string]string{}, entries: map[[2]string]*db.NegotiationSupersession{}}
}

// claim records that negotiationID is now the request whose values stand for
// each of fields, superseding whichever request held them before.
func (d *discardLog) claim(negotiationID string, fields []string) {
	for _, field := range fields {
		if previous, held := d.holder[field]; held && previous != negotiationID {
			key := [2]string{previous, negotiationID}
			entry, seen := d.entries[key]
			if !seen {
				entry = &db.NegotiationSupersession{NegotiationID: previous, SupersededByID: negotiationID}
				d.entries[key] = entry
				d.order = append(d.order, key)
			}
			entry.Fields = append(entry.Fields, field)
		}
		d.holder[field] = negotiationID
	}
}

func (d *discardLog) records() []db.NegotiationSupersession {
	if len(d.order) == 0 {
		return nil
	}
	records := make([]db.NegotiationSupersession, 0, len(d.order))
	for _, key := range d.order {
		records = append(records, *d.entries[key])
	}
	return records
}
