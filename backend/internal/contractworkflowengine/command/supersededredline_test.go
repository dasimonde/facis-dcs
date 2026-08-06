package command

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"io"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/contractworkflowengine/datatype/eventtype"
	"digital-contracting-service/internal/contractworkflowengine/datatype/negotiationtaskstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
)

// The audit-truth defect: a round with two accepted change requests keeps only
// the later one's values, but both decision rows read ACCEPTED for good, so
// the record claimed a party had agreed to content the contract never carried.
// Folding the round must therefore leave the discard on the losing request and
// on the trail.
func TestMergingARoundRecordsTheAcceptedRedlineItDiscarded(t *testing.T) {
	first := datatype.JSON(`{"name":"Title the first negotiator proposed"}`)
	second := datatype.JSON(`{"name":"Title the second negotiator proposed"}`)
	negotiations := &negotiationRepoFake{
		negotiations: true,
		accepted: map[int][]db.NegotiationChangeData{
			2: {
				{ID: "negotiation-first", ChangeRequest: &first},
				{ID: "negotiation-second", ChangeRequest: &second},
			},
		},
	}
	tasks := &negotiationTaskRepoFake{rows: []db.NegotiationTaskData{{
		DID:             taskContractDID,
		ContractVersion: 2,
		Negotiator:      localPeerDID,
		State:           negotiationtaskstate.Open.String(),
	}}}
	contracts := &taskContractRepoFake{process: inNegotiation(2), stored: taskStoredContract()}
	outbox := &recordingOutbox{}
	handler := &Submitter{
		DB:          outboxRecordingDB(t, outbox),
		CRepo:       contracts,
		NRepo:       negotiations,
		NTRepo:      tasks,
		DIDDocument: *taskTestDIDDocument(t),
	}

	require.NoError(t, handler.Handle(context.Background(), negotiationSubmitCmd()))

	require.Equal(t, "Title the second negotiator proposed", *contracts.updates[0].Name,
		"last-accepted-wins is the merge semantics and stays that way")
	require.Equal(t, []db.NegotiationSupersession{{
		NegotiationID:  "negotiation-first",
		SupersededByID: "negotiation-second",
		Fields:         []string{"name"},
	}}, negotiations.superseded,
		"the discarded request must carry the annotation, since its decision still reads ACCEPTED")

	recorded := outbox.eventOfType(t, eventtype.NegotiationChangeSuperseded.String())
	require.Equal(t, 2, recorded.ContractVersion, "the round the discarded request was accepted on")
	require.Equal(t, 3, recorded.MergedVersion, "the version that does not carry it")
	require.Equal(t, "negotiation-second", recorded.Superseded[0].SupersededByID)
}

// The annotation is a statement about a specific loss, so a round whose
// accepted requests do not contend must produce none — and no event either.
func TestMergingARoundRecordsNoDiscardWhenNothingWasOverwritten(t *testing.T) {
	only := datatype.JSON(`{"name":"The one title anyone proposed"}`)
	negotiations := &negotiationRepoFake{
		negotiations: true,
		accepted:     map[int][]db.NegotiationChangeData{2: {{ID: "negotiation-only", ChangeRequest: &only}}},
	}
	tasks := &negotiationTaskRepoFake{rows: []db.NegotiationTaskData{{
		DID:             taskContractDID,
		ContractVersion: 2,
		Negotiator:      localPeerDID,
		State:           negotiationtaskstate.Open.String(),
	}}}
	outbox := &recordingOutbox{}
	handler := &Submitter{
		DB:          outboxRecordingDB(t, outbox),
		CRepo:       &taskContractRepoFake{process: inNegotiation(2), stored: taskStoredContract()},
		NRepo:       negotiations,
		NTRepo:      tasks,
		DIDDocument: *taskTestDIDDocument(t),
	}

	require.NoError(t, handler.Handle(context.Background(), negotiationSubmitCmd()))

	require.Empty(t, negotiations.superseded)
	require.NotContains(t, outbox.eventTypes(), eventtype.NegotiationChangeSuperseded.String())
}

// recordingOutbox keeps the (event_type, event_data) pairs event.Create wrote,
// which is the audit trail's only entry point from a command handler. It is
// its own database driver: the repositories are faked, so the outbox INSERT is
// the only statement that reaches the driver at all.
type recordingOutbox struct {
	rows [][]driver.Value
}

func (o *recordingOutbox) eventTypes() []string {
	types := make([]string, 0, len(o.rows))
	for _, row := range o.rows {
		types = append(types, row[1].(string))
	}
	return types
}

func (o *recordingOutbox) eventOfType(t *testing.T, eventType string) contractevents.NegotiationChangeSupersededEvent {
	t.Helper()
	for _, row := range o.rows {
		if row[1].(string) != eventType {
			continue
		}
		var evt contractevents.NegotiationChangeSupersededEvent
		require.NoError(t, json.Unmarshal(row[2].([]byte), &evt))
		return evt
	}
	t.Fatalf("no %s event reached the outbox, only %v", eventType, o.eventTypes())
	return contractevents.NegotiationChangeSupersededEvent{}
}

// outboxRecordingDB is taskTestDB with the outbox INSERT captured rather than
// swallowed.
func outboxRecordingDB(t *testing.T, outbox *recordingOutbox) *sqlx.DB {
	t.Helper()
	raw := sql.OpenDB(outbox)
	t.Cleanup(func() { require.NoError(t, raw.Close()) })
	return sqlx.NewDb(raw, "postgres")
}

func (o *recordingOutbox) Connect(context.Context) (driver.Conn, error) { return o, nil }
func (o *recordingOutbox) Driver() driver.Driver                        { return o }
func (o *recordingOutbox) Open(string) (driver.Conn, error)             { return o, nil }
func (o *recordingOutbox) Prepare(string) (driver.Stmt, error)          { return o, nil }
func (o *recordingOutbox) Close() error                                 { return nil }
func (o *recordingOutbox) Begin() (driver.Tx, error)                    { return o, nil }
func (o *recordingOutbox) NumInput() int                                { return -1 }
func (o *recordingOutbox) Query([]driver.Value) (driver.Rows, error)    { return nil, io.EOF }
func (o *recordingOutbox) Commit() error                                { return nil }
func (o *recordingOutbox) Rollback() error                              { return nil }

func (o *recordingOutbox) Exec(args []driver.Value) (driver.Result, error) {
	o.rows = append(o.rows, args)
	return driver.RowsAffected(1), nil
}
