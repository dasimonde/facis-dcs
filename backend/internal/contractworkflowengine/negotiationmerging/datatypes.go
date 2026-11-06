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

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ErrInvalidChangeRequest refuses a change request that is neither a
// free-text note nor a ChangeRequest object.
var ErrInvalidChangeRequest = errors.New("invalid change request: send a JSON string note or an object with name, description, contract_data, start_date, exp_date (RFC 3339), exp_notice_period or exp_policy")

type ChangeRequest struct {
	Name            *string          `json:"name"`
	Description     *string          `json:"description"`
	ContractData    *json.RawMessage `json:"contract_data"`
	StartDate       *string          `json:"start_date"`
	ExpDate         *string          `json:"exp_date,omitempty"`
	ExpNoticePeriod *int             `json:"exp_notice_period,omitempty"`
	ExpPolicy       *string          `json:"exp_policy,omitempty"`
}

// Decode reads a change request. A JSON string (or null) is a free-text note
// for the negotiation audit trail: note is true and the merge changes nothing
// for it. Any other value must be a ChangeRequest object with RFC 3339 dates.
func Decode(raw []byte) (change ChangeRequest, note bool, err error) {
	var text *string
	if json.Unmarshal(raw, &text) == nil {
		return ChangeRequest{}, true, nil
	}
	if err := json.Unmarshal(raw, &change); err != nil {
		return ChangeRequest{}, false, fmt.Errorf("%w: %v", ErrInvalidChangeRequest, err)
	}
	for _, date := range []*string{change.StartDate, change.ExpDate} {
		if date == nil {
			continue
		}
		if _, err := time.Parse(time.RFC3339, *date); err != nil {
			return ChangeRequest{}, false, fmt.Errorf("%w: %v", ErrInvalidChangeRequest, err)
		}
	}
	return change, false, nil
}
