package command

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// CWE-7: a ship that reaches a terminated, expired or withdrawn local copy is
// refused. The copy keeps its content, version and PDF, and a task its
// termination left decided is not opened again.
func TestReceiveShipIntoAClosedCopyIsRefused(t *testing.T) {
	for _, state := range []string{"TERMINATED", "WITHDRAWN", "EXPIRED"} {
		t.Run(state, func(t *testing.T) {
			receiver, database := signingConflictReceiver(t, unsignedBase)
			ctx := context.Background()
			_, err := database.Exec(`UPDATE contracts SET state = $2 WHERE did = $1`, taskContractDID, state)
			require.NoError(t, err)
			if state == "EXPIRED" {
				// EXPIRED is the effective state of a contract past its end date.
				_, err = database.Exec(`UPDATE contracts SET state = 'ACTIVE', exp_date = now() - INTERVAL '1 day' WHERE did = $1`, taskContractDID)
				require.NoError(t, err)
			}
			_, err = database.Exec(`INSERT INTO contract_negotiation_task (did, state, negotiator, created_by, contract_version)
				SELECT did, 'ACCEPTED', $2, $2, contract_version FROM contracts WHERE did = $1`, taskContractDID, localPeerDID)
			require.NoError(t, err)
			var before struct {
				Version int    `db:"contract_version"`
				Data    string `db:"contract_data"`
			}
			require.NoError(t, database.Get(&before, `SELECT contract_version, contract_data::text AS contract_data FROM contracts WHERE did = $1`, taskContractDID))

			counteroffer := shipOf(withSignature(unsignedBase, "A"))
			counteroffer.Payload = []byte(`{"@id":"` + taskContractDID + `","dcs:name":"counteroffer"}`)
			require.ErrorIs(t, receiver.Handle(ctx, counteroffer), ErrClosedCopy)

			var after struct {
				Version int    `db:"contract_version"`
				Data    string `db:"contract_data"`
			}
			require.NoError(t, database.Get(&after, `SELECT contract_version, contract_data::text AS contract_data FROM contracts WHERE did = $1`, taskContractDID))
			require.Equal(t, before, after, "the ship changed the closed copy")
			require.Equal(t, unsignedBase, localPDF(t, receiver), "the ship replaced the PDF of the closed copy")
			var open int
			require.NoError(t, database.Get(&open, `SELECT count(*) FROM contract_negotiation_task WHERE did = $1 AND state = 'OPEN'`, taskContractDID))
			require.Zero(t, open, "the ship opened a negotiation task on the closed copy")
		})
	}
}
