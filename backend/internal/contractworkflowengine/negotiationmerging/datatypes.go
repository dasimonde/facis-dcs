// Package negotiationmerging folds the accepted change requests of a
// negotiation round into a new contract version (see MergeChangeRequests),
// triggered from command.Submitter once every negotiation task is closed.
// Conflicting changes are resolved by last-write-wins in persistence order —
// there is no explicit conflict detection between contradictory requests
// from different negotiators.
package negotiationmerging

import "encoding/json"

type ChangeRequest struct {
	Name            *string          `json:"name"`
	Description     *string          `json:"description"`
	ContractData    *json.RawMessage `json:"contract_data"`
	StartDate       *string          `json:"start_date"`
	ExpDate         *string          `json:"exp_date,omitempty"`
	ExpNoticePeriod *int             `json:"exp_notice_period,omitempty"`
	ExpPolicy       *string          `json:"exp_policy,omitempty"`
}
