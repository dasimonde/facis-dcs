package workflowgate

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/validation"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

type claimDriver struct{ state *claimDriverState }
type claimDriverState struct {
	mu    sync.Mutex
	runID string
}
type claimConn struct{ state *claimDriverState }
type claimRows struct {
	columns []string
	values  [][]driver.Value
}

func (d claimDriver) Open(string) (driver.Conn, error) { return &claimConn{state: d.state}, nil }
func (c *claimConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare is unsupported")
}
func (c *claimConn) Close() error { return nil }
func (c *claimConn) Begin() (driver.Tx, error) {
	return nil, errors.New("transactions are unsupported")
}
func (c *claimConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if !strings.Contains(query, "INSERT INTO pac_workflow_gate_runs") {
		return nil, errors.New("unexpected exec")
	}
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	if c.state.runID != "" {
		return driver.RowsAffected(0), nil
	}
	c.state.runID = args[0].Value.(string)
	return driver.RowsAffected(1), nil
}
func (c *claimConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	switch {
	case strings.Contains(query, "FROM pac_workflow_gate_runs"):
		return &claimRows{
			columns: []string{"run_id", "correlation_id", "snapshot_id", "contract_did", "contract_version",
				"contract_state", "contract_updated_at", "gate", "status", "content_hash", "effective_shapes",
				"profile_version", "continuation_json"},
			values: [][]driver.Value{{
				c.state.runID, "66d1a314-c32f-4259-af22-62808c993bcc", "sha256:snapshot",
				"did:web:example.test:contract", int64(1), "DRAFT", time.Now().UTC(),
				"submission", "DISPATCHING", "sha256:content", []byte(`["shape"]`), int64(1), []byte(`{}`),
			}},
		}, nil
	case strings.Contains(query, "pac_workflow_gate_review_decisions"),
		strings.Contains(query, "pac_workflow_gate_continuation_attempts"):
		return &claimRows{columns: []string{"status"}}, nil
	default:
		return nil, errors.New("unexpected query")
	}
}
func (r *claimRows) Columns() []string { return r.columns }
func (r *claimRows) Close() error      { return nil }
func (r *claimRows) Next(dest []driver.Value) error {
	if len(r.values) == 0 {
		return io.EOF
	}
	copy(dest, r.values[0])
	r.values = r.values[1:]
	return nil
}

func validGateRequest() Request {
	return Request{
		ContractVersion: ContractVersion, CorrelationID: "66d1a314-c32f-4259-af22-62808c993bcc",
		SnapshotID: "sha256:snapshot", Gate: "submission",
		Snapshot: Snapshot{ContractDID: "did:web:example.test:contract", ContractVersion: 1,
			ContentHash: "sha256:content", EffectiveShapes: []string{"https://dcs.test/semantic/shapes/facis-dcs?version=1"}, ProfileVersion: 1},
		LocalEvaluation: LocalEvaluation{Result: "PASSED", Findings: []validation.PolicyFinding{}},
	}
}

func validGateResponse(request Request) Response {
	return Response{
		ContractVersion: ContractVersion, CorrelationID: request.CorrelationID,
		SnapshotID: request.SnapshotID, Gate: request.Gate, Result: "PASSED",
		Findings: []Finding{}, ExecutorID: "test", ExecutorVersion: "1",
		ExecutedAt: time.Now().UTC().Format(time.RFC3339),
	}
}

func TestHTTPClientDispatchesExactlyOnceAndValidatesCorrelation(t *testing.T) {
	request := validGateRequest()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		require.Equal(t, "Bearer secret", r.Header.Get("Authorization"))
		_ = json.NewEncoder(w).Encode(validGateResponse(request))
	}))
	defer server.Close()
	client, err := NewHTTPClient(server.URL, "secret", time.Second)
	require.NoError(t, err)
	response, _, err := client.Run(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, "PASSED", response.Result)
	require.EqualValues(t, 1, calls.Load())
}

func TestHTTPClientFailsClosedWithoutRetry(t *testing.T) {
	request := validGateRequest()
	tests := []struct {
		name   string
		mutate func(*Response)
	}{
		{"contract version mismatch", func(response *Response) { response.ContractVersion = "unsupported" }},
		{"correlation mismatch", func(response *Response) { response.CorrelationID = "mismatch" }},
		{"snapshot mismatch", func(response *Response) { response.SnapshotID = "mismatch" }},
		{"gate mismatch", func(response *Response) { response.Gate = "deployment" }},
		{"invalid result", func(response *Response) { response.Result = "UNKNOWN" }},
		{"missing findings", func(response *Response) { response.Findings = nil }},
		{"missing executor identity", func(response *Response) { response.ExecutorID = "" }},
		{"invalid execution time", func(response *Response) { response.ExecutedAt = "tomorrow" }},
		{"invalid finding", func(response *Response) {
			response.Findings = []Finding{{RuleID: "policy", Result: "UNKNOWN", Reason: "invalid"}}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				response := validGateResponse(request)
				test.mutate(&response)
				_ = json.NewEncoder(w).Encode(response)
			}))
			defer server.Close()
			client, err := NewHTTPClient(server.URL, "", time.Second)
			require.NoError(t, err)
			_, _, err = client.Run(context.Background(), request)
			require.Error(t, err)
			require.EqualValues(t, 1, calls.Load())
		})
	}
}

func TestHTTPClientRejectsProtocolFailuresWithoutRetry(t *testing.T) {
	request := validGateRequest()
	tests := []struct {
		name    string
		timeout time.Duration
		handler http.HandlerFunc
	}{
		{
			name: "non 2xx",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "upstream failure", http.StatusBadGateway)
			},
		},
		{
			name: "malformed JSON",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte("{"))
			},
		},
		{
			name: "unknown response member",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				response := validGateResponse(request)
				raw, _ := json.Marshal(response)
				raw = append(raw[:len(raw)-1], []byte(`,"unexpected":true}`)...)
				_, _ = w.Write(raw)
			},
		},
		{
			name: "trailing JSON",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(validGateResponse(request))
				_, _ = w.Write([]byte(`{}`))
			},
		},
		{
			name:    "timeout",
			timeout: 10 * time.Millisecond,
			handler: func(w http.ResponseWriter, _ *http.Request) {
				time.Sleep(50 * time.Millisecond)
				_ = json.NewEncoder(w).Encode(validGateResponse(request))
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				test.handler(w, r)
			}))
			defer server.Close()
			timeout := test.timeout
			if timeout == 0 {
				timeout = time.Second
			}
			client, err := NewHTTPClient(server.URL, "", timeout)
			require.NoError(t, err)
			_, _, err = client.Run(context.Background(), request)
			require.Error(t, err)
			require.EqualValues(t, 1, calls.Load())
		})
	}
}

func TestHTTPClientRequiresCompleteConfiguration(t *testing.T) {
	_, err := NewHTTPClient("", "", time.Second)
	require.Error(t, err)
	_, err = NewHTTPClient("https://executor.example/run", "", 0)
	require.Error(t, err)
}

// A deferred finding is a rule the target system decides at use-time (ADR-33).
// It must not hold the workflow — a context operand would then refuse every
// contract that uses one — and it must not let the run read as PASSED either.
func TestLocalResultSeparatesDeferredFromPassed(t *testing.T) {
	finding := func(severity string) validation.PolicyFinding {
		return validation.PolicyFinding{RuleID: "FACIS-RULE", Severity: severity}
	}

	require.Equal(t, "PASSED", resultFromLocal([]validation.PolicyFinding{finding(validation.SeveritySatisfied)}))
	require.Equal(t, "NOT_EVALUATED", resultFromLocal([]validation.PolicyFinding{
		finding(validation.SeveritySatisfied), finding(validation.SeverityDeferred),
	}))
	require.Equal(t, "REVIEW", resultFromLocal([]validation.PolicyFinding{
		finding(validation.SeverityDeferred), finding(validation.SeverityWarning),
	}))
	require.Equal(t, "BLOCKED", resultFromLocal([]validation.PolicyFinding{
		finding(validation.SeverityDeferred), finding(validation.SeverityError),
	}))
	require.Equal(t, "SUCCESS", resultStatus("NOT_EVALUATED", Response{Result: "PASSED", Findings: []Finding{}}))
}

// The contract's own dcs:policies are enforced where the contract is
// committed to — ValidateContractPolicySatisfaction at approve.go and
// signingmanagement apply.go — not at every transition. A gate that blocked on
// them refused submission, offer and negotiation-settle, which the SLA
// federation vertical performs deliberately with an out-of-boundary
// counter-offer, and replaced the rule-naming refusal with a generic one.
func TestLocalResultLeavesContractODRLToItsOwnEnforcementPoint(t *testing.T) {
	contractODRL := validation.PolicyFinding{
		RuleID: "FACIS-CONTRACT-ODRL-POLICY", Severity: validation.SeverityError,
		Source: validation.SourceContractODRL,
	}
	hubShapes := validation.PolicyFinding{
		RuleID: "title-InConstraintComponent", Severity: validation.SeverityError,
		Source: validation.SourceHubShapes,
	}
	policySet := validation.PolicyFinding{
		RuleID: "FACIS-BLACKLIST-COUNTRY", Severity: validation.SeverityError,
		Source: validation.SourcePolicySetODRL,
	}

	require.Equal(t, "PASSED", resultFromLocal([]validation.PolicyFinding{contractODRL}))
	require.Empty(t, blockingFindings([]validation.PolicyFinding{contractODRL}))

	// Everything the gate IS the enforcement point for still blocks, including
	// an untagged finding from a caller that predates the source tagging.
	require.Equal(t, "BLOCKED", resultFromLocal([]validation.PolicyFinding{contractODRL, hubShapes}))
	require.Equal(t, "BLOCKED", resultFromLocal([]validation.PolicyFinding{policySet}))
	require.Equal(t, "BLOCKED", resultFromLocal([]validation.PolicyFinding{
		{RuleID: "FACIS-UNTAGGED", Severity: validation.SeverityError},
	}))

	// A warning on the contract's own policies does not hold the transition
	// for human review either.
	contractODRL.Severity = validation.SeverityWarning
	require.Equal(t, "PASSED", resultFromLocal([]validation.PolicyFinding{contractODRL}))
}

func TestBlockedGateNamesTheFindingThatBlockedIt(t *testing.T) {
	blocked := &LocalEvaluationBlockedError{Findings: blockingFindings([]validation.PolicyFinding{
		{RuleID: "FACIS-SATISFIED", Severity: validation.SeveritySatisfied, Message: "holds"},
		{
			RuleID: "title-InConstraintComponent", Severity: validation.SeverityError,
			Message: "value is not in the allowed list", Source: validation.SourceHubShapes,
		},
		{
			RuleID: "FACIS-CONTRACT-ODRL-POLICY", Severity: validation.SeverityError,
			Message: "not the gate's call", Source: validation.SourceContractODRL,
		},
	})}

	require.Equal(t, []string{"title-InConstraintComponent: value is not in the allowed list"}, blocked.Reasons())
	require.Equal(t,
		"workflow gate blocked: local Semantic Hub evaluation blocked the transition: "+
			"title-InConstraintComponent: value is not in the allowed list",
		(&BlockedError{Status: "BLOCKED", Cause: blocked}).Error())

	var unwrapped *LocalEvaluationBlockedError
	require.True(t, errors.As(error(&BlockedError{Status: "BLOCKED", Cause: blocked}), &unwrapped))
	require.Len(t, unwrapped.Findings, 1)

	// A blocked run with no nameable finding must still not claim a failure
	// that did not happen: the evaluation succeeded and found something.
	require.Equal(t, "local Semantic Hub evaluation blocked the transition",
		(&LocalEvaluationBlockedError{}).Error())
}

func TestResultPrecedence(t *testing.T) {
	require.Equal(t, "SUCCESS", resultStatus("PASSED", Response{Result: "PASSED", Findings: []Finding{}}))
	require.Equal(t, "REVIEW", resultStatus("REVIEW", Response{Result: "PASSED", Findings: []Finding{}}))
	require.Equal(t, "REVIEW", resultStatus("PASSED", Response{Result: "REVIEW", Findings: []Finding{}}))
	require.Equal(t, "BLOCKED", resultStatus("PASSED", Response{Result: "REVIEW", Findings: []Finding{{Result: "FAILED"}}}))
}

func TestExpectedUpdatedAtUsesWorkflowConcurrencyPrecision(t *testing.T) {
	stored := time.Date(2026, 7, 29, 4, 43, 40, 807299000, time.UTC)
	contentUpdated := time.Date(2026, 7, 29, 4, 43, 40, 757162000, time.UTC)
	apiToken := time.Date(2026, 7, 29, 4, 43, 40, 0, time.UTC)

	// RFC3339 formatting omits .807299 and returns the start of the same second.
	// This is the same revision, not a stale token.
	require.True(t, acceptsExpectedUpdatedAt(stored, contentUpdated, apiToken))
	require.True(t, acceptsExpectedUpdatedAt(stored, contentUpdated, time.Time{}))
	require.False(t, acceptsExpectedUpdatedAt(stored, contentUpdated, apiToken.Add(-time.Second)))
	require.False(t, acceptsExpectedUpdatedAt(stored, contentUpdated, apiToken.Add(time.Second)))
}

func TestExpectedUpdatedAtRejectsRegressedStoredRevision(t *testing.T) {
	// A transaction-start timestamp must never make a later committed revision
	// appear older than an already issued second-precision API token. The
	// contracts trigger prevents this state; the gate must not compensate for it.
	apiToken := time.Date(2026, 7, 29, 4, 57, 40, 0, time.UTC)
	stored := time.Date(2026, 7, 29, 4, 57, 39, 860950000, time.UTC)
	contentUpdated := time.Date(2026, 7, 29, 4, 57, 39, 699275000, time.UTC)

	require.False(t, acceptsExpectedUpdatedAt(stored, contentUpdated, apiToken))
}

func TestExpectedUpdatedAtIgnoresTechnicalTimestampOnlyAdvance(t *testing.T) {
	contentUpdated := time.Date(2026, 7, 29, 12, 34, 56, 789123000, time.UTC)
	technicalUpdated := contentUpdated.Add(30 * time.Second)
	clientToken := contentUpdated

	require.True(t, acceptsExpectedUpdatedAt(technicalUpdated, contentUpdated, clientToken))
	require.False(t, acceptsExpectedUpdatedAt(
		technicalUpdated,
		contentUpdated.Add(time.Second),
		clientToken,
	))
}

func TestSnapshotQueryUsesViewsThatActuallyExposeRequiredColumns(t *testing.T) {
	require.Contains(t, contractSnapshotQuery, "FROM contracts_effective AS effective")
	require.Contains(t, contractSnapshotQuery, "JOIN contracts_effective_process_data AS process_data")
	require.Contains(t, contractSnapshotQuery, "process_data.content_updated_at")
	require.Contains(t, contractSnapshotQuery, "effective.contract_data")

	// content_updated_at is intentionally not part of contracts_effective;
	// qualifying it through that view would regress to a runtime pq error.
	require.False(t, strings.Contains(contractSnapshotQuery, "effective.content_updated_at"))
}

func TestSnapshotRevalidationRemainsExact(t *testing.T) {
	updated := time.Date(2026, 7, 29, 12, 34, 56, 789123000, time.UTC)
	contentUpdated := updated.Add(-time.Minute)
	content := map[string]any{"@id": "did:web:example.test:contract", "state": "draft"}
	expected := Snapshot{
		ContractVersion:  4,
		State:            "draft",
		UpdatedAt:        updated,
		ContentUpdatedAt: contentUpdated,
		ContentHash:      hashJSON(content),
	}

	require.True(t, sameSnapshotRevision(expected, 4, "draft", contentUpdated, content))
	require.False(t, sameSnapshotRevision(expected, 5, "draft", contentUpdated, content))
	require.False(t, sameSnapshotRevision(expected, 4, "submitted", contentUpdated, content))
	require.False(t, sameSnapshotRevision(expected, 4, "draft", contentUpdated.Add(time.Microsecond), content))
	require.False(t, sameSnapshotRevision(expected, 4, "draft", contentUpdated, map[string]any{
		"@id": "did:web:example.test:contract", "state": "changed",
	}))
}

func TestSnapshotRevalidationAllowsTechnicalUpdatedAtBump(t *testing.T) {
	contentUpdated := time.Date(2026, 7, 29, 12, 34, 0, 0, time.UTC)
	content := map[string]any{"@id": "did:web:example.test:contract", "value": "unchanged"}
	expected := Snapshot{
		ContractVersion:  4,
		State:            "REVIEWED",
		UpdatedAt:        contentUpdated.Add(time.Second),
		ContentUpdatedAt: contentUpdated,
		ContentHash:      hashJSON(content),
	}

	// updated_at is deliberately not part of the consequential comparison.
	// verifySnapshot returns its freshly read value separately as the command
	// token after this predicate succeeds.
	require.True(t, sameSnapshotRevision(expected, 4, "REVIEWED", contentUpdated, content))
}

func TestSnapshotIdentitySeparatesConsequentialStateTransitions(t *testing.T) {
	base := Snapshot{
		ContractDID:     "did:web:example.test:contract",
		ContractVersion: 4,
		State:           "DRAFT",
		ContentHash:     "sha256:content",
		EffectiveShapes: []string{"https://dcs.test/semantic/shapes/facis-dcs?version=1"},
		ProfileVersion:  1,
	}

	// A retry against the identical immutable state must reuse the same gate
	// run, preserving at-most-once dispatch.
	require.Equal(t, snapshotIdentity(base), snapshotIdentity(base))

	// Submission is overloaded across multiple state-machine transitions.
	// Identical content/version in the next state is a new consequential
	// snapshot and must therefore receive a distinct run.
	negotiation := base
	negotiation.State = "NEGOTIATION"
	require.NotEqual(t, snapshotIdentity(base), snapshotIdentity(negotiation))
}

func TestConcurrentClaimDispatchesExactlyOnceAndReusesSameRun(t *testing.T) {
	state := &claimDriverState{}
	driverName := "workflow-gate-claim-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	sql.Register(driverName, claimDriver{state: state})
	rawDB, err := sql.Open(driverName, "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = rawDB.Close() })
	db := sqlx.NewDb(rawDB, driverName)
	coordinator := &Coordinator{DB: db}
	request := validGateRequest()
	request.Snapshot.State = "DRAFT"
	request.Snapshot.UpdatedAt = time.Now().UTC()
	input := Input{Gate: request.Gate, ContractDID: request.Snapshot.ContractDID}

	type result struct {
		runID    string
		inserted bool
		err      error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	var dispatches atomic.Int32
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			runID, inserted, existing, err := coordinator.beginRun(context.Background(), input, request)
			if inserted {
				dispatches.Add(1) // the only branch allowed to call Client.Run
			} else if err == nil {
				runID = existing.RunID
			}
			results <- result{runID: runID, inserted: inserted, err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	var got []result
	for result := range results {
		got = append(got, result)
	}
	require.Len(t, got, 2)
	require.NoError(t, got[0].err)
	require.NoError(t, got[1].err)
	require.EqualValues(t, 1, dispatches.Load())
	require.NotEqual(t, got[0].inserted, got[1].inserted)
	require.NotEmpty(t, got[0].runID)
	require.Equal(t, got[0].runID, got[1].runID)
}

// closeoutDriver honours the context the way a real database driver does, and
// records the statements that actually reached it.
type closeoutDriver struct{ state *closeoutState }
type closeoutState struct {
	mu       sync.Mutex
	executed []string
}
type closeoutConn struct{ state *closeoutState }

func (d closeoutDriver) Open(string) (driver.Conn, error) { return &closeoutConn{state: d.state}, nil }
func (c *closeoutConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare is unsupported")
}
func (c *closeoutConn) Close() error { return nil }
func (c *closeoutConn) Begin() (driver.Tx, error) {
	return nil, errors.New("transactions are unsupported")
}
func (c *closeoutConn) ExecContext(ctx context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	c.state.executed = append(c.state.executed, query)
	return driver.RowsAffected(1), nil
}

// A gate dispatch usually fails because the caller went away — an HTTP client
// timeout, a cancelled request. Closing the run out on that same dead context
// wrote nothing, so the row stayed DISPATCHING; since (snapshot_id,gate) is
// unique, every later transition of that contract then read the abandoned run
// and was refused with 409 "does not permit transition", permanently.
func TestRunIsClosedOutEvenWhenTheCallerIsGone(t *testing.T) {
	state := &closeoutState{}
	driverName := "workflow-gate-closeout-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	sql.Register(driverName, closeoutDriver{state: state})
	rawDB, err := sql.Open(driverName, "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = rawDB.Close() })
	coordinator := &Coordinator{DB: sqlx.NewDb(rawDB, driverName)}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	require.NoError(t, coordinator.finish(cancelled, uuid.NewString(), "BLOCKED", validGateRequest(), nil,
		errors.New("executor dispatch failed: context canceled")))

	state.mu.Lock()
	defer state.mu.Unlock()
	require.Len(t, state.executed, 1)
	require.Contains(t, state.executed[0], "UPDATE pac_workflow_gate_runs")
	require.Contains(t, state.executed[0], "status='DISPATCHING'")
}

func TestReviewedContinuationRefreshesTimestampForUnchangedSnapshot(t *testing.T) {
	content := map[string]any{
		"@id":                  "did:web:example.test:contract",
		"dcs:effectiveShapes":  []any{"https://dcs.test/shapes?version=1"},
		"dcterms:conformsTo":   map[string]any{"@id": "https://dcs.test/profile?version=1"},
		"technicalBookkeeping": "unchanged",
	}
	original := time.Date(2026, 7, 29, 12, 0, 0, 1000, time.UTC)
	current := original.Add(30 * time.Second)
	run := Run{
		ContractDID:       "did:web:example.test:contract",
		ContractVersion:   4,
		ContractState:     "NEGOTIATION",
		ContractUpdatedAt: original,
		ContentHash:       hashJSON(content),
	}

	refreshed, err := refreshReviewedSnapshot(run, 4, "NEGOTIATION", current, content)
	require.NoError(t, err)
	require.Equal(t, current, refreshed.ContractUpdatedAt)

	var continuationTimestamp time.Time
	resume := func(_ context.Context, continued Run) error {
		continuationTimestamp = continued.ContractUpdatedAt
		return nil
	}
	require.NoError(t, resume(context.Background(), refreshed))
	require.Equal(t, current, continuationTimestamp)
}

func TestReviewedContinuationRejectsConsequentialSnapshotChanges(t *testing.T) {
	content := map[string]any{"@id": "did:web:example.test:contract", "value": "original"}
	run := Run{
		ContractVersion: 4,
		ContractState:   "NEGOTIATION",
		ContentHash:     hashJSON(content),
	}
	now := time.Now().UTC()
	tests := []struct {
		name    string
		version int
		state   string
		content map[string]any
	}{
		{"version changed", 5, "NEGOTIATION", content},
		{"state changed", 4, "SUBMITTED", content},
		{"content changed", 4, "NEGOTIATION", map[string]any{
			"@id": "did:web:example.test:contract", "value": "changed",
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := refreshReviewedSnapshot(run, test.version, test.state, now, test.content)
			require.Error(t, err)
		})
	}
}

func TestReviewDecisionReplayCannotReplacePersistedDecision(t *testing.T) {
	persisted := ReviewDecision{
		Decision:      "approve",
		Justification: "reviewed semantic warning",
		DecidedBy:     "did:web:compliance.example",
	}

	require.True(t, sameReviewDecision(
		persisted, persisted.Decision, persisted.Justification, persisted.DecidedBy,
	))
	require.False(t, sameReviewDecision(
		persisted, "reject", persisted.Justification, persisted.DecidedBy,
	))
	require.False(t, sameReviewDecision(
		persisted, persisted.Decision, "replacement rationale", persisted.DecidedBy,
	))
	require.False(t, sameReviewDecision(
		persisted, persisted.Decision, persisted.Justification, "did:web:other.example",
	))
}

// A contract derived from a federated template declares the canonical DCS
// envelope graph AND the shape libraries its author modelled its data against,
// so sh:shapesGraph is a list. Collapsing that list to a single anchor is what
// blocked every federated offer.
func TestShapesBundleConsistencyReadsTheMultiValuedDeclaration(t *testing.T) {
	canonical := "https://dcs.test/semantic/shapes/facis-dcs?version=4"
	library := "https://peer.test/semantic/shapes/partner-library?version=9"
	content := map[string]any{
		"sh:shapesGraph": []any{
			map[string]any{"@id": canonical},
			map[string]any{"@id": library},
		},
		"dcs:effectiveShapes": []any{
			map[string]any{"@id": canonical},
			map[string]any{"@id": "https://dcs.test/semantic/shapes/clause-catalog?version=2"},
			map[string]any{"@id": library},
		},
	}
	require.NoError(t, requireConsistentShapesBundle(content))

	// The single-anchor form every contract without libraries carries.
	require.NoError(t, requireConsistentShapesBundle(map[string]any{
		"sh:shapesGraph":      map[string]any{"@id": canonical},
		"dcs:effectiveShapes": []any{map[string]any{"@id": canonical}},
	}))
}

// A hub asset is identified by its entry name and version, never by the URL it
// is served from: a contract derived from a template imported off another
// instance's federated catalogue names the very same graph under the publishing
// instance's hostname. Comparing IRIs would block every federated contract.
func TestShapesBundleConsistencyIdentifiesGraphsByNameAndVersionNotHost(t *testing.T) {
	require.NoError(t, requireConsistentShapesBundle(map[string]any{
		"sh:shapesGraph": []any{
			map[string]any{"@id": "https://dcs-ionos.test/api/semantic/shapes/facis-dcs?version=1"},
			map[string]any{"@id": "https://dcs-ionos.test/api/semantic/shapes/sla-hosting?version=2"},
		},
		"dcs:effectiveShapes": []any{
			map[string]any{"@id": "https://dcs-osc.test/api/semantic/shapes/facis-dcs?version=1"},
			map[string]any{"@id": "https://dcs-osc.test/api/semantic/shapes/clause-catalog?version=1"},
			map[string]any{"@id": "https://dcs-osc.test/api/semantic/shapes/sla-hosting?version=2"},
		},
	}))
}

func TestShapesBundleConsistencyFailsClosed(t *testing.T) {
	canonical := "https://dcs.test/semantic/shapes/facis-dcs?version=4"
	for name, content := range map[string]map[string]any{
		"no declaration at all": {
			"dcs:effectiveShapes": []any{map[string]any{"@id": canonical}},
		},
		"canonical pinned at another version than the bundle": {
			"sh:shapesGraph":      map[string]any{"@id": "https://dcs.test/semantic/shapes/facis-dcs?version=1"},
			"dcs:effectiveShapes": []any{map[string]any{"@id": canonical}},
		},
		"a library the bundle does not carry": {
			"sh:shapesGraph": []any{
				map[string]any{"@id": canonical},
				map[string]any{"@id": "https://peer.test/semantic/shapes/partner-library?version=9"},
			},
			"dcs:effectiveShapes": []any{map[string]any{"@id": canonical}},
		},
		"a library the bundle carries at another version": {
			"sh:shapesGraph": []any{
				map[string]any{"@id": canonical},
				map[string]any{"@id": "https://peer.test/semantic/shapes/partner-library?version=9"},
			},
			"dcs:effectiveShapes": []any{
				map[string]any{"@id": canonical},
				map[string]any{"@id": "https://dcs.test/semantic/shapes/partner-library?version=3"},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			require.Error(t, requireConsistentShapesBundle(content))
		})
	}
}

// The live regression, end to end over one contract document: an OSC contract
// drawn from a template imported off the IONOS federated catalogue could not be
// submitted. Its pin is written at creation, a client save replaces the
// document with the one its editor assembled — which models none of these
// properties — and the gate then has nothing to build a snapshot from.
func TestClientReplacedDocumentKeepsTheSnapshotBuildable(t *testing.T) {
	imported, err := datatype.NewJSON(map[string]any{
		"@id": "https://dcs-osc.test/api/contract/f30a2f9e",
		"sh:shapesGraph": []any{
			// The publishing instance's own anchors, as an imported template
			// carries them.
			map[string]any{"@id": "https://dcs-ionos.test/api/semantic/shapes/facis-dcs?version=1"},
			map[string]any{"@id": "https://dcs-ionos.test/api/semantic/shapes/facis-sla-hosting?version=2"},
		},
	})
	require.NoError(t, err)
	created, err := validation.PinSemanticBundle(&imported,
		"https://dcs-osc.test/api/semantic/context/facis-dcs?version=1",
		"https://dcs-osc.test/api/semantic/shapes/facis-dcs?version=1",
		[]string{
			"https://dcs-osc.test/api/semantic/shapes/facis-dcs?version=1",
			"https://dcs-osc.test/api/semantic/shapes/clause-catalog?version=1",
		},
		nil,
		"https://dcs-osc.test/api/semantic/profile/facis.sla.basic?version=1")
	require.NoError(t, err)
	require.NoError(t, requireConsistentShapesBundle(decodeContent(t, created)))

	// What the editor posts back: no bundle at all, and a canonical anchor
	// still naming the instance the template came from.
	saved, err := datatype.NewJSON(map[string]any{
		"@id":            "https://dcs-osc.test/api/contract/f30a2f9e",
		"@context":       "https://w3id.org/facis/dcs/context/v1",
		"sh:shapesGraph": map[string]any{"@id": "https://dcs-ionos.test/api/semantic/shapes/facis-dcs?version=1"},
	})
	require.NoError(t, err)
	stripped := decodeContent(t, &saved)
	require.Nil(t, stripped["dcs:effectiveShapes"], "the client document is the one that carries no pin")

	carried, err := validation.CarrySemanticBundle(created, &saved)
	require.NoError(t, err)
	require.NoError(t, requireConsistentShapesBundle(decodeContent(t, carried)))
}

func decodeContent(t *testing.T, raw *datatype.JSON) map[string]any {
	t.Helper()
	var content map[string]any
	require.NoError(t, json.Unmarshal(*raw, &content))
	return content
}
