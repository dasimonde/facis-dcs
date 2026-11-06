package service

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	goa "goa.design/goa/v3/pkg"

	cwe "digital-contracting-service/gen/contract_workflow_engine"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/userrole"
	basedb "digital-contracting-service/internal/base/db"
	"digital-contracting-service/internal/middleware"
)

func lifecycleEntry(eventType string, createdAt time.Time, body string) datatype.AuditLogEntry {
	return datatype.AuditLogEntry{EventType: eventType, CreatedAt: createdAt, EventData: json.RawMessage(body)}
}

func TestLifecycleEventsKeepsOnlyTransitionsWithActorAndTime(t *testing.T) {
	start := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	entries := []datatype.AuditLogEntry{
		lifecycleEntry("RETRIEVE_CONTRACT_BY_ID", start.Add(9*time.Hour), `{"retrieved_by":"Org C","occurred_at":"2026-10-01T17:00:00Z"}`),
		lifecycleEntry("AUDIT_CONTRACT", start.Add(8*time.Hour), `{"audited_by":"Auditor Org"}`),
		lifecycleEntry("CONTRACT_ACCESS_DENIED", start.Add(7*time.Hour), `{"retrieved_by":"Org D"}`),
		lifecycleEntry("RECORD_EVIDENCE", start.Add(6*time.Hour), `{"recorded_by":"Org A"}`),
		lifecycleEntry("ACTIVATE_CONTRACT", start.Add(10*time.Hour), `{"activated_by":"dcs-target-t-1","occurred_at":"2026-10-01T18:00:00Z","previous_state":"SIGNED","new_state":"ACTIVE","correlation_id":"corr-1"}`),
		lifecycleEntry("CONTRACT_EXPIRED", start.Add(5*time.Hour), `{"occurred_at":"2026-10-01T13:00:00Z"}`),
		lifecycleEntry("APPROVE_CONTRACT", start.Add(3*time.Hour), `{"erased":true}`),
		lifecycleEntry("SUBMIT_CONTRACT", start, `{"submitted_by":"Org B","occurred_at":"2026-10-01T10:30:00.5Z","previous_state":"SUBMITTED","new_state":"REVIEWED","comments":["confidential finding"]}`),
		lifecycleEntry("CREATE_CONTRACT", start, `{"created_by":"Org A","occurred_at":"2026-10-01T08:00:00Z","contract_data":{"secret":true}}`),
		lifecycleEntry("APPLIED_SIGNATURE", start.Add(4*time.Hour), `{"applied_by":"Org B","occurred_at":"2026-10-01T12:00:00Z"}`),
	}

	got := lifecycleEvents(entries)

	str := func(s string) *string { return &s }
	require.Equal(t, []*cwe.ContractLifecycleEvent{
		{EventType: "ACTIVATE_CONTRACT", Actor: str("dcs-target-t-1"), OccurredAt: "2026-10-01T18:00:00Z", PreviousState: str("SIGNED"), NewState: str("ACTIVE")},
		{EventType: "CONTRACT_EXPIRED", OccurredAt: "2026-10-01T13:00:00Z"},
		{EventType: "APPLIED_SIGNATURE", Actor: str("Org B"), OccurredAt: "2026-10-01T12:00:00Z"},
		{EventType: "APPROVE_CONTRACT", OccurredAt: "2026-10-01T11:00:00Z"},
		{EventType: "SUBMIT_CONTRACT", Actor: str("Org B"), OccurredAt: "2026-10-01T10:30:00Z", PreviousState: str("SUBMITTED"), NewState: str("REVIEWED")},
		{EventType: "CREATE_CONTRACT", Actor: str("Org A"), OccurredAt: "2026-10-01T08:00:00Z"},
	}, got)
}

// CWE-14: a termination notice the counterparty refused for good shows on the
// contract, naming the counterparty and not the refusal text.
func TestLifecycleShowsARefusedTerminationNotice(t *testing.T) {
	at := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)
	got := lifecycleEvents([]datatype.AuditLogEntry{lifecycleEntry("PEER_EXCHANGE_REFUSED", at,
		`{"exchange":"terminate","peer_did":"did:web:peer.example","reason":"not_found","occurred_at":"2026-10-01T08:00:00Z"}`)})

	peer := "did:web:peer.example"
	require.Equal(t, []*cwe.ContractLifecycleEvent{{EventType: "PEER_EXCHANGE_REFUSED", Actor: &peer, OccurredAt: "2026-10-01T08:00:00Z"}}, got)
}

func TestLifecycleAuthorizesContractPartyOnly(t *testing.T) {
	database := sqlx.NewDb(sql.OpenDB(lifecycleContractConnector{}), "postgres")
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	repo := &lifecycleTrailRepo{}
	engine := &contractWorkflowEnginesrvc{
		DB:           database,
		DIDDocument:  *jadesTestDIDDocument(t, "local"),
		ATrailReader: base.AuditTrailReader{ARepo: repo},
	}
	asOrg := func(org string) context.Context {
		return middleware.InjectAuthContext(context.Background(), []string{userrole.ContractObserver.String()}, "holder", org)
	}

	_, err := engine.Lifecycle(asOrg("Org C"), &cwe.LifecyclePayload{Did: "did:contract:1"})
	var named *goa.ServiceError
	require.True(t, errors.As(err, &named), "got %v", err)
	require.Equal(t, "forbidden", named.Name)
	require.Empty(t, repo.components, "a foreign organization must not reach the audit trail")

	events, err := engine.Lifecycle(asOrg("Org B"), &cwe.LifecyclePayload{Did: "did:contract:1"})
	require.NoError(t, err)
	require.Empty(t, events)
	require.Equal(t, []string{"CONTRACT_WORKFLOW_ENGINE", "SIGNATURE_MANAGEMENT", "CONTRACT_STORAGE_ARCHIVE"}, repo.components)
}

type lifecycleTrailRepo struct {
	basedb.AuditTrailRepository
	components []string
}

func (r *lifecycleTrailRepo) ReadLogCID(_ context.Context, _ *sqlx.Tx, component, _ string) (*string, error) {
	r.components = append(r.components, component)
	return nil, nil
}

// lifecycleContractConnector serves one contract created by Org A with Org B as a party.
type lifecycleContractConnector struct{}

func (c lifecycleContractConnector) Connect(context.Context) (driver.Conn, error) { return c, nil }
func (c lifecycleContractConnector) Driver() driver.Driver                        { return c }
func (c lifecycleContractConnector) Open(string) (driver.Conn, error)             { return c, nil }
func (c lifecycleContractConnector) Prepare(string) (driver.Stmt, error)          { return c, nil }
func (lifecycleContractConnector) Close() error                                   { return nil }
func (c lifecycleContractConnector) Begin() (driver.Tx, error)                    { return c, nil }
func (c lifecycleContractConnector) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return c, nil
}
func (lifecycleContractConnector) Commit() error   { return nil }
func (lifecycleContractConnector) Rollback() error { return nil }
func (lifecycleContractConnector) NumInput() int   { return -1 }
func (lifecycleContractConnector) Exec([]driver.Value) (driver.Result, error) {
	return driver.RowsAffected(0), nil
}
func (lifecycleContractConnector) Query([]driver.Value) (driver.Rows, error) {
	return &lifecycleContractRows{}, nil
}

type lifecycleContractRows struct{ done bool }

func (*lifecycleContractRows) Columns() []string {
	return []string{"origin", "created_by", "contract_data"}
}
func (*lifecycleContractRows) Close() error { return nil }
func (r *lifecycleContractRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	dest[0], dest[1], dest[2] = "", "Org A", []byte(`{"dcs:parties":[{"dcs:legalName":"Org B"}]}`)
	return nil
}
