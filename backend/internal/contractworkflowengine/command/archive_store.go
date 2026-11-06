package command

import (
	"encoding/json"
	"fmt"

	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
)

// ArchiveManagerStoreSource marks an archive entry an Archive Manager stored
// (DCS-IR-CSA-02), as opposed to the entry the signing workflow writes.
const ArchiveManagerStoreSource = "ARCHIVE_MANAGER_STORE"

// archivableStates are the final states an Archive Manager can archive a
// contract in.
var archivableStates = map[string]bool{
	contractstate.Signed.String():     true,
	contractstate.Terminated.String(): true,
	contractstate.Expired.String():    true,
}

// BuildManualArchiveEntry freezes the current version of a SIGNED, TERMINATED
// or EXPIRED contract for an archive entry an Archive Manager stores, with the
// signatures this instance applied, the signatures a peer applied and the
// version history (ReadHistoryByDID) (DCS-FR-CSA-06, DCS-FR-CSA-07,
// DCS-FR-CSA-08, DCS-FR-CWE-20). A contract that carries no signature is not
// archived: the signing workflow ran its compliance gates and the
// synchronization check of the signed PDF before any signature.
func BuildManualArchiveEntry(contract *db.Contract, history []db.ContractHistory, storedBy string, signatures []ArchiveSigningEvidence, peerSignatures json.RawMessage) (db.ContractArchiveEntry, error) {
	if contract == nil {
		return db.ContractArchiveEntry{}, fmt.Errorf("contract is required")
	}
	if !archivableStates[contract.State] {
		return db.ContractArchiveEntry{}, fmt.Errorf("contract %s is %s; only SIGNED, TERMINATED or EXPIRED contracts can be archived", contract.DID, contract.State)
	}
	if len(signatures) == 0 && len(peerSignatures) == 0 {
		return db.ContractArchiveEntry{}, fmt.Errorf("contract %s was never signed; only a signed contract can be archived", contract.DID)
	}
	return buildSignedArchiveEntry(contract, history, storedBy, ArchiveManagerStoreSource, signatures, peerSignatures)
}
