package command

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/pgtest"
	cwepg "digital-contracting-service/internal/contractworkflowengine/db/pg"
)

func TestEvidenceIntegrityAndLimits(t *testing.T) {
	hash := sha256.Sum256([]byte("original"))
	e := Evidence{EvidenceID: "evidence", DID: "contract", Content: []byte("original"), SHA256: hex.EncodeToString(hash[:]), Filename: "name", Description: "description"}
	require.NoError(t, e.Verify("contract", "evidence"))
	require.Error(t, e.Verify("another-contract", "evidence"))
	require.Error(t, e.Verify("contract", "another-evidence"))
	e.Content = []byte("changed")
	require.Error(t, e.Verify("contract", "evidence"))
	t.Setenv("DCS_EVIDENCE_MAX_TEXT_BYTES", "4")
	require.ErrorIs(t, e.Validate(), ErrInvalidEvidence)
}

// DCS-IR-CWE-12: the Contract Creator attaches evidence to the draft; after
// that, storing evidence is the Manager's. The recorder has no artifact store
// here, so a call that passes the role check fails on the store.
func TestTheCreatorRecordsEvidenceOnlyOnADraftPostgres(t *testing.T) {
	database := pgtest.Open(t)
	_, err := database.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version) VALUES
		('draft',$1,'Org A','DRAFT','tpl',1),('rejected',$1,'Org A','REJECTED','tpl',1),('active',$1,'Org A','ACTIVE','tpl',1)`, localPeerDID)
	require.NoError(t, err)
	recorder := &EvidenceRecorder{DB: database, CRepo: &cwepg.PostgresContractRepo{}, DIDDocument: *taskTestDIDDocument(t)}
	record := func(did string, role userrole.UserRole) error {
		_, err := recorder.Handle(context.Background(), RecordEvidenceCmd{
			DID: did, RecordedBy: "Org A", UpdatedAt: time.Now().Add(time.Hour), CauserDID: localPeerDID, UserRoles: userrole.UserRoles{role},
			Evidence: Evidence{Content: []byte("proof"), Filename: "proof.txt", Description: "proof"},
		})
		return err
	}

	require.ErrorIs(t, record("active", userrole.ContractCreator), ErrEvidenceCreatorDraftOnly)
	for _, did := range []string{"draft", "rejected"} {
		require.ErrorContains(t, record(did, userrole.ContractCreator), "artifact store unavailable", did)
	}
	require.ErrorContains(t, record("active", userrole.ContractManager), "artifact store unavailable")
}
