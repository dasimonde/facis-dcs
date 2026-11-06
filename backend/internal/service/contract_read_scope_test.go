package service

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	pac "digital-contracting-service/gen/process_audit_and_compliance"
	sm "digital-contracting-service/gen/signature_management"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/middleware"
	"digital-contracting-service/internal/processauditandcompliance/workflowgate"
)

// contractRowsDriver answers the trusted contract lookup and the workflow-gate run read.
// Transactions fail, so a read that passes authorization stops with an internal error.
type contractRowsDriver struct{ peer string }
type contractRowsConn struct{ peer string }
type fakeRows struct {
	columns []string
	values  [][]driver.Value
}

var contractOwners = map[string]string{"own": "Org A", "foreign": "Org B"}

func (d contractRowsDriver) Open(string) (driver.Conn, error) { return contractRowsConn(d), nil }
func (contractRowsConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare is unsupported")
}
func (contractRowsConn) Close() error { return nil }
func (contractRowsConn) Begin() (driver.Tx, error) {
	return nil, errors.New("transactions are unsupported")
}
func (c contractRowsConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	switch {
	case strings.Contains(query, "FROM contracts_effective WHERE did"):
		rows := &fakeRows{columns: []string{"origin", "created_by", "contract_data"}}
		if owner, ok := contractOwners[args[0].Value.(string)]; ok {
			rows.values = [][]driver.Value{{c.peer, owner, []byte(`{}`)}}
		}
		return rows, nil
	case strings.Contains(query, "FROM pac_workflow_gate_runs"):
		return &fakeRows{
			columns: []string{"run_id", "correlation_id", "snapshot_id", "contract_did", "contract_version",
				"contract_state", "contract_updated_at", "gate", "status", "content_hash", "effective_shapes",
				"profile_version", "continuation_json", "local_evaluation", "response_json", "created_at"},
			values: [][]driver.Value{{args[0].Value, "correlation", "snapshot", args[0].Value, int64(1), "DRAFT", time.Now().UTC(),
				"submission", "PASSED", "sha256:content", []byte(`[]`), int64(1), []byte(`{}`), []byte(`{}`), []byte(`{}`), time.Now().UTC()}},
		}, nil
	case strings.Contains(query, "pac_workflow_gate_review_decisions"),
		strings.Contains(query, "pac_workflow_gate_continuation_attempts"):
		return &fakeRows{columns: []string{"status"}}, nil
	}
	return nil, errors.New("unexpected query")
}
func (r *fakeRows) Columns() []string { return r.columns }
func (r *fakeRows) Close() error      { return nil }
func (r *fakeRows) Next(dest []driver.Value) error {
	if len(r.values) == 0 {
		return io.EOF
	}
	copy(dest, r.values[0])
	r.values = r.values[1:]
	return nil
}

func contractRowsDB(t *testing.T, peer string) *sqlx.DB {
	name := "contract-rows-" + t.Name()
	sql.Register(name, contractRowsDriver{peer: peer})
	raw, err := sql.Open(name, "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = raw.Close() })
	return sqlx.NewDb(raw, "postgres")
}

func errorName(err error) string {
	var named interface{ ErrorName() string }
	if errors.As(err, &named) {
		return named.ErrorName()
	}
	return ""
}

func TestClauseReportIsLimitedToContractParties(t *testing.T) {
	document := jadesTestDIDDocument(t, "local")
	peer, err := document.GetID()
	require.NoError(t, err)
	service := &signatureManagementsrvc{DB: contractRowsDB(t, peer), DIDDocument: *document}
	ctx := middleware.InjectAuthContext(context.Background(), []string{"Contract Observer"}, "holder", "Org A")

	_, err = service.ClauseReport(ctx, &sm.SMClauseReportRequest{Did: "foreign"})
	require.Equal(t, "forbidden", errorName(err))
	_, err = service.ClauseReport(ctx, &sm.SMClauseReportRequest{Did: "missing"})
	require.Equal(t, "not_found", errorName(err))
	_, err = service.ClauseReport(ctx, &sm.SMClauseReportRequest{Did: "own"})
	require.Equal(t, "internal_error", errorName(err))
}

func TestWorkflowGateRunIsLimitedToContractParties(t *testing.T) {
	peer := "did:web:local"
	database := contractRowsDB(t, peer)
	service := &processAuditAndCompliancesrvc{
		DB:           database,
		ATrailReader: base.AuditTrailReader{Artifacts: artifactstore.New(nil, nil, nil, peer, "", nil)},
		WorkflowGate: &workflowgate.Coordinator{DB: database},
	}
	ctx := middleware.InjectAuthContext(context.Background(), []string{"Compliance Officer"}, "holder", "Org A")

	_, err := service.WorkflowGateRun(ctx, &pac.WorkflowGateRunPayload{RunID: "foreign"})
	require.Equal(t, "forbidden", errorName(err))
	run, err := service.WorkflowGateRun(ctx, &pac.WorkflowGateRunPayload{RunID: "own"})
	require.NoError(t, err)
	require.Equal(t, "own", run.ContractDid)
}
