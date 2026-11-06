package negotiationmerging

import (
	"bytes"
	"encoding/json"

	"digital-contracting-service/internal/base/jades"
)

// ContractDataAlreadyEquals reports whether a change request's document is the
// same (by settlement digest) as the formal base — typical of mid-flight
// contracts under the old Propose-applied-immediately behavior.
// Folding such a request again is a no-op for the document; callers skip the
// replace so a legacy already-materialized redline cannot clobber a later
// accepted document via last-accepted-wins ordering quirks.
func ContractDataAlreadyEquals(base map[string]any, rawChange json.RawMessage) (bool, error) {
	changeData, err := mergeContractDataChange(base, rawChange)
	if err != nil {
		return false, err
	}

	baseJSON, err := json.Marshal(base)
	if err != nil {
		return false, err
	}

	changeJSON, err := json.Marshal(changeData)
	if err != nil {
		return false, err
	}

	if bytes.Equal(baseJSON, changeJSON) {
		return true, nil
	}

	baseDigest, err := jades.ContractDocumentDigest(baseJSON)
	if err != nil {
		return false, err
	}

	changeDigest, err := jades.ContractDocumentDigest(changeJSON)
	if err != nil {
		return false, err
	}

	return baseDigest == changeDigest, nil
}
