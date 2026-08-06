// Package negotiationmerging folds the accepted change requests of a
// negotiation round into a new contract version (see MergeChangeRequests),
// triggered from command.Submitter once every negotiation task is closed.
// Conflicting changes are resolved by last-accepted-wins in proposal order
// (created_at, id) — there is no explicit conflict detection between
// contradictory requests from different negotiators.
//
// Last-accepted-wins means acceptance and effect come apart: an accepted
// request whose fields a later accepted request also set contributes nothing
// to the merged version, while its decision row still reads ACCEPTED. The
// fold therefore reports every such discard back to the caller, which records
// it against the losing request and on the audit trail, so the contract's
// record never presents an accepted-and-dropped redline as content the
// parties agreed the contract would carry.
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
