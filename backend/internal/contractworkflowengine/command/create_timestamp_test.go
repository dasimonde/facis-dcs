package command

import (
	"encoding/json"
	"testing"
	"time"

	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"

	"github.com/stretchr/testify/require"
)

func TestCreateContractEventTimestampIsNonZeroUTCAndRFC3339(t *testing.T) {
	contract, evt := withCreationTimestamp(db.Contract{}, contractevents.CreateEvent{})

	require.False(t, contract.CreatedAt.IsZero(), "contract created_at must not be the zero time")
	require.False(t, evt.OccurredAt.IsZero(), "CREATE_CONTRACT occurred_at must not be the zero time")
	require.Equal(t, contract.CreatedAt, evt.OccurredAt, "contract and event must receive the same creation instant")
	require.Equal(t, time.UTC, contract.CreatedAt.Location())
	require.Equal(t, time.UTC, evt.OccurredAt.Location())

	encoded, err := json.Marshal(evt)
	require.NoError(t, err)
	require.Contains(t, string(encoded), contract.CreatedAt.Format(time.RFC3339Nano))
}
