package workflowgate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"digital-contracting-service/internal/base/validation"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

const ContractVersion = "facis-pac-workflow-gate/v1"

const contractSnapshotQuery = `
        SELECT effective.contract_version, effective.state, effective.updated_at,
               process_data.content_updated_at, effective.contract_data
        FROM contracts_effective AS effective
        JOIN contracts_effective_process_data AS process_data
          ON process_data.did = effective.did
        WHERE effective.did=$1`

type Snapshot struct {
	ContractDID      string         `json:"contract_did"`
	ContractVersion  int            `json:"contract_version"`
	State            string         `json:"state"`
	UpdatedAt        time.Time      `json:"updated_at"`
	ContentUpdatedAt time.Time      `json:"-"`
	ContentHash      string         `json:"content_hash"`
	Content          map[string]any `json:"content"`
	EffectiveShapes  []string       `json:"effective_shapes"`
	ProfileVersion   int            `json:"profile_version"`
}

type LocalEvaluation struct {
	Result   string                     `json:"result"`
	Findings []validation.PolicyFinding `json:"findings"`
}

type Request struct {
	ContractVersion string          `json:"contract_version"`
	CorrelationID   string          `json:"correlation_id"`
	SnapshotID      string          `json:"snapshot_id"`
	Gate            string          `json:"gate"`
	Snapshot        Snapshot        `json:"snapshot"`
	LocalEvaluation LocalEvaluation `json:"local_evaluation"`
}

type Finding struct {
	RuleID   string `json:"rule_id"`
	Result   string `json:"result"`
	Reason   string `json:"reason"`
	Severity string `json:"severity,omitempty"`
}

type Response struct {
	ContractVersion string    `json:"contract_version"`
	CorrelationID   string    `json:"correlation_id"`
	SnapshotID      string    `json:"snapshot_id"`
	Gate            string    `json:"gate"`
	Result          string    `json:"result"`
	Findings        []Finding `json:"findings"`
	ExecutorID      string    `json:"executor_id"`
	ExecutorVersion string    `json:"executor_version"`
	ExecutedAt      string    `json:"executed_at"`
}

type Client interface {
	Run(context.Context, Request) (Response, []byte, error)
}

type HTTPClient struct {
	url, bearer string
	client      *http.Client
}

func NewHTTPClient(url, bearer string, timeout time.Duration) (*HTTPClient, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return nil, errors.New("workflow-gate executor URL is required")
	}
	if timeout <= 0 {
		return nil, errors.New("workflow-gate executor timeout must be positive")
	}
	return &HTTPClient{url: url, bearer: strings.TrimSpace(bearer), client: &http.Client{Timeout: timeout}}, nil
}

func (c *HTTPClient) Run(ctx context.Context, request Request) (Response, []byte, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return Response{}, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return Response{}, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearer)
	}
	res, err := c.client.Do(req) // exactly one attempt; the stable run owns retries
	if err != nil {
		return Response{}, nil, fmt.Errorf("call workflow-gate executor: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(res.Body, (1<<20)+1))
	if err != nil {
		return Response{}, nil, err
	}
	if len(raw) > 1<<20 {
		return Response{}, nil, errors.New("workflow-gate response exceeds 1 MiB")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return Response{}, nil, fmt.Errorf("workflow-gate executor returned HTTP %d", res.StatusCode)
	}
	var result Response
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return Response{}, nil, fmt.Errorf("decode workflow-gate response: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return Response{}, nil, errors.New("workflow-gate response contains trailing JSON")
	}
	if err := validateResponse(request, result); err != nil {
		return Response{}, nil, err
	}
	return result, raw, nil
}

func validateResponse(request Request, response Response) error {
	if response.ContractVersion != ContractVersion || response.ContractVersion != request.ContractVersion {
		return errors.New("workflow-gate contract version mismatch")
	}
	if response.CorrelationID != request.CorrelationID {
		return errors.New("workflow-gate correlation mismatch")
	}
	if response.SnapshotID != request.SnapshotID || response.Gate != request.Gate {
		return errors.New("workflow-gate snapshot or gate mismatch")
	}
	if response.Findings == nil {
		return errors.New("workflow-gate findings must be an array")
	}
	if strings.TrimSpace(response.ExecutorID) == "" || strings.TrimSpace(response.ExecutorVersion) == "" {
		return errors.New("workflow-gate executor identity is incomplete")
	}
	if _, err := time.Parse(time.RFC3339, response.ExecutedAt); err != nil {
		return errors.New("workflow-gate execution time is invalid")
	}
	if response.Result != "PASSED" && response.Result != "REVIEW" && response.Result != "FAILED" {
		return errors.New("workflow-gate result is invalid")
	}
	for i, finding := range response.Findings {
		if strings.TrimSpace(finding.RuleID) == "" || strings.TrimSpace(finding.Reason) == "" ||
			(finding.Result != "PASSED" && finding.Result != "REVIEW" && finding.Result != "FAILED") {
			return fmt.Errorf("workflow-gate finding %d is invalid", i)
		}
	}
	return nil
}

type Input struct {
	Gate              string
	ContractDID       string
	ExpectedUpdatedAt time.Time
	Requester         string
	Roles             []string
	Continuation      map[string]any
}

type BlockedError struct {
	RunID  string
	Status string
	Cause  error
}

func (e *BlockedError) Error() string {
	if e.Cause == nil {
		return "workflow gate blocked"
	}
	return "workflow gate blocked: " + e.Cause.Error()
}
func (e *BlockedError) Unwrap() error { return e.Cause }

type ReviewDecision struct {
	DecisionID, RunID, Decision, Justification, DecidedBy string
	DecidedAt                                             time.Time
}

type Run struct {
	RunID, CorrelationID, SnapshotID, ContractDID, ContractState, Gate, Status, ContentHash string
	ContractVersion                                                                         int
	ContractUpdatedAt                                                                       time.Time
	EffectiveShapes                                                                         []string
	ProfileVersion                                                                          int
	Continuation                                                                            map[string]any
	Review                                                                                  *ReviewDecision
	ContinuationStatus                                                                      string
}

type Coordinator struct {
	DB     *sqlx.DB
	Client Client
	mu     sync.RWMutex
	resume map[string]func(context.Context, Run) error
}

func (c *Coordinator) SetReviewContinuation(gate string, fn func(context.Context, Run) error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.resume == nil {
		c.resume = make(map[string]func(context.Context, Run) error)
	}
	c.resume[gate] = fn
}

func (c *Coordinator) Execute(ctx context.Context, input Input) (string, bool, error) {
	runID, reused, _, err := c.ExecuteSnapshot(ctx, input)
	return runID, reused, err
}

func (c *Coordinator) ExecuteSnapshot(ctx context.Context, input Input) (string, bool, time.Time, error) {
	if c == nil || c.DB == nil || c.Client == nil {
		return "", false, time.Time{}, &BlockedError{Status: "BLOCKED", Cause: errors.New("workflow gate is not configured")}
	}
	snapshot, snapshotID, err := c.snapshot(ctx, input)
	if err != nil {
		runID, persistErr := c.persistClosedRun(ctx, input, Snapshot{ContractDID: input.ContractDID}, "", "BLOCKED", LocalEvaluation{Result: "BLOCKED"}, err)
		if persistErr != nil {
			return "", false, time.Time{}, &BlockedError{Status: "BLOCKED", Cause: errors.Join(err, persistErr)}
		}
		return runID, false, time.Time{}, &BlockedError{RunID: runID, Status: "BLOCKED", Cause: err}
	}
	localFindings, err := validation.AuditContractContent(ctx, snapshot.Content, nil, validation.ContractContentAuditMetadata{
		ContractDID: snapshot.ContractDID, ContractVersion: fmt.Sprint(snapshot.ContractVersion), AuditedBy: input.Requester,
	})
	if err != nil {
		runID, persistErr := c.persistClosedRun(ctx, input, snapshot, snapshotID, "BLOCKED", LocalEvaluation{Result: "BLOCKED"}, err)
		if persistErr != nil {
			err = errors.Join(err, persistErr)
		}
		return runID, false, snapshot.UpdatedAt, &BlockedError{RunID: runID, Status: "BLOCKED", Cause: err}
	}
	local := LocalEvaluation{Result: resultFromLocal(localFindings), Findings: localFindings}
	if local.Result == "BLOCKED" {
		runID, persistErr := c.persistClosedRun(ctx, input, snapshot, snapshotID, "BLOCKED", local, errors.New("local Semantic Hub evaluation failed"))
		if persistErr != nil {
			return "", false, snapshot.UpdatedAt, persistErr
		}
		return runID, false, snapshot.UpdatedAt, &BlockedError{RunID: runID, Status: "BLOCKED", Cause: errors.New("local Semantic Hub evaluation failed")}
	}

	request := Request{ContractVersion: ContractVersion, CorrelationID: uuid.NewString(), SnapshotID: snapshotID, Gate: input.Gate, Snapshot: snapshot, LocalEvaluation: local}
	runID, inserted, existing, err := c.beginRun(ctx, input, request)
	if err != nil {
		return "", false, snapshot.UpdatedAt, &BlockedError{Status: "BLOCKED", Cause: err}
	}
	if !inserted {
		if existing.Status == "SUCCESS" ||
			(existing.Status == "REVIEW" && existing.Review != nil &&
				existing.Review.Decision == "approve" && existing.ContinuationStatus == "SUCCESS") {
			return existing.RunID, true, snapshot.UpdatedAt, nil
		}
		return existing.RunID, true, snapshot.UpdatedAt, &BlockedError{RunID: existing.RunID, Status: existing.Status, Cause: errors.New("existing workflow gate run does not permit transition")}
	}

	response, raw, dispatchErr := c.Client.Run(ctx, request)
	if dispatchErr != nil {
		_ = c.finish(ctx, runID, "BLOCKED", request, nil, dispatchErr)
		return runID, false, snapshot.UpdatedAt, &BlockedError{RunID: runID, Status: "BLOCKED", Cause: dispatchErr}
	}
	status := resultStatus(local.Result, response)
	currentUpdatedAt, verifyErr := c.verifySnapshot(ctx, snapshot)
	if verifyErr != nil {
		status = "BLOCKED"
		dispatchErr = verifyErr
	} else {
		// Technical PDF/audit writers may advance updated_at while the
		// consequential snapshot remains identical. Return the freshly read
		// token to the command that follows this gate.
		snapshot.UpdatedAt = currentUpdatedAt
	}
	if err := c.finish(ctx, runID, status, request, raw, dispatchErr); err != nil {
		return runID, false, snapshot.UpdatedAt, &BlockedError{RunID: runID, Status: "BLOCKED", Cause: err}
	}
	if status != "SUCCESS" {
		return runID, false, snapshot.UpdatedAt, &BlockedError{RunID: runID, Status: status, Cause: errors.New("workflow gate result is " + status)}
	}
	return runID, false, snapshot.UpdatedAt, nil
}

func resultFromLocal(findings []validation.PolicyFinding) string {
	result := "PASSED"
	for _, finding := range findings {
		switch strings.ToLower(strings.TrimSpace(finding.Severity)) {
		case "error", "critical", "blocking", "violation":
			return "BLOCKED"
		case "warning", "warn", "review":
			result = "REVIEW"
		}
	}
	return result
}

func resultStatus(local string, response Response) string {
	status := "SUCCESS"
	if local == "REVIEW" || response.Result == "REVIEW" {
		status = "REVIEW"
	}
	if response.Result == "FAILED" {
		status = "BLOCKED"
	}
	for _, finding := range response.Findings {
		if finding.Result == "FAILED" {
			return "BLOCKED"
		}
		if finding.Result == "REVIEW" && status != "BLOCKED" {
			status = "REVIEW"
		}
	}
	return status
}

func (c *Coordinator) snapshot(ctx context.Context, input Input) (Snapshot, string, error) {
	var raw []byte
	var snapshot Snapshot
	err := c.DB.QueryRowxContext(ctx, contractSnapshotQuery, input.ContractDID).
		Scan(&snapshot.ContractVersion, &snapshot.State, &snapshot.UpdatedAt, &snapshot.ContentUpdatedAt, &raw)
	if err != nil {
		return snapshot, "", fmt.Errorf("read contract snapshot: %w", err)
	}
	snapshot.ContractDID = input.ContractDID
	// Workflow APIs expose updated_at truncated to RFC3339 second precision.
	// Compare both DB boundaries in exactly that representation; the exact DB
	// revision captured below is still revalidated after executor dispatch.
	if !acceptsExpectedUpdatedAt(snapshot.UpdatedAt, snapshot.ContentUpdatedAt, input.ExpectedUpdatedAt) {
		return snapshot, "", errors.New("contract changed before workflow gate")
	}
	if err := json.Unmarshal(raw, &snapshot.Content); err != nil {
		return snapshot, "", fmt.Errorf("decode contract snapshot: %w", err)
	}
	snapshot.ContentHash = hashJSON(snapshot.Content)
	rawRefs, ok := snapshot.Content["dcs:effectiveShapes"].([]any)
	if !ok || len(rawRefs) == 0 {
		return snapshot, "", errors.New("immutable workflow snapshot has no effective shapes")
	}
	for _, rawRef := range rawRefs {
		ref := anchor(rawRef)
		if ref == "" {
			return snapshot, "", errors.New("immutable workflow snapshot has invalid effective shape")
		}
		snapshot.EffectiveShapes = append(snapshot.EffectiveShapes, ref)
	}
	canonical := anchor(snapshot.Content["sh:shapesGraph"])
	if canonical == "" || canonical != snapshot.EffectiveShapes[0] {
		return snapshot, "", errors.New("immutable workflow snapshot has missing or mismatched canonical shapes")
	}
	profile := anchor(snapshot.Content["dcterms:conformsTo"])
	snapshot.ProfileVersion = versionFromAnchor(profile)
	if snapshot.ProfileVersion <= 0 {
		return snapshot, "", errors.New("immutable workflow snapshot has no validation profile")
	}
	return snapshot, snapshotIdentity(snapshot), nil
}

func (c *Coordinator) verifySnapshot(ctx context.Context, expected Snapshot) (time.Time, error) {
	var version int
	var state string
	var updated time.Time
	var contentUpdated time.Time
	var raw []byte
	if err := c.DB.QueryRowxContext(ctx, contractSnapshotQuery, expected.ContractDID).
		Scan(&version, &state, &updated, &contentUpdated, &raw); err != nil {
		return time.Time{}, err
	}
	var content map[string]any
	if err := json.Unmarshal(raw, &content); err != nil {
		return time.Time{}, err
	}
	if !sameSnapshotRevision(expected, version, state, contentUpdated, content) {
		return time.Time{}, errors.New("contract changed during workflow gate dispatch")
	}
	return updated, nil
}

func acceptsExpectedUpdatedAt(stored, contentUpdatedAt, expected time.Time) bool {
	if expected.IsZero() {
		return true
	}
	// updated_at also moves for PDF/audit bookkeeping. A caller token that is
	// older than that technical timestamp is still current when it is not older
	// than the last actual contract_data change.
	expected = expected.Truncate(time.Second)
	stored = stored.Truncate(time.Second)
	contentUpdatedAt = contentUpdatedAt.Truncate(time.Second)
	return !expected.After(stored) && !expected.Before(contentUpdatedAt)
}

func sameSnapshotRevision(expected Snapshot, version int, state string, contentUpdatedAt time.Time, content map[string]any) bool {
	return version == expected.ContractVersion &&
		state == expected.State &&
		contentUpdatedAt.Equal(expected.ContentUpdatedAt) &&
		hashJSON(content) == expected.ContentHash
}

func snapshotIdentity(snapshot Snapshot) string {
	return hashJSON(map[string]any{
		"did":              snapshot.ContractDID,
		"version":          snapshot.ContractVersion,
		"state":            snapshot.State,
		"content_hash":     snapshot.ContentHash,
		"effective_shapes": snapshot.EffectiveShapes,
		"profile_version":  snapshot.ProfileVersion,
	})
}

func (c *Coordinator) beginRun(ctx context.Context, input Input, request Request) (string, bool, Run, error) {
	runID := uuid.NewString()
	snapshotJSON, _ := json.Marshal(request.Snapshot)
	shapesJSON, _ := json.Marshal(request.Snapshot.EffectiveShapes)
	localJSON, _ := json.Marshal(request.LocalEvaluation)
	continuationJSON, _ := json.Marshal(input.Continuation)
	requestJSON, _ := json.Marshal(request)
	result, err := c.DB.ExecContext(ctx, `
        INSERT INTO pac_workflow_gate_runs
          (run_id,correlation_id,snapshot_id,contract_did,contract_version,contract_state,
           contract_updated_at,gate,status,content_hash,snapshot_json,effective_shapes,
           profile_version,local_evaluation,continuation_json,request_json)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'DISPATCHING',$9,$10::jsonb,$11::jsonb,$12,$13::jsonb,$14::jsonb,$15::jsonb)
        ON CONFLICT (snapshot_id,gate) DO NOTHING`,
		runID, request.CorrelationID, request.SnapshotID, request.Snapshot.ContractDID,
		request.Snapshot.ContractVersion, request.Snapshot.State, request.Snapshot.UpdatedAt,
		request.Gate, request.Snapshot.ContentHash, string(snapshotJSON), string(shapesJSON),
		request.Snapshot.ProfileVersion, string(localJSON), string(continuationJSON), string(requestJSON))
	if err != nil {
		return "", false, Run{}, err
	}
	count, _ := result.RowsAffected()
	if count == 1 {
		return runID, true, Run{}, nil
	}
	existing, err := c.Read(ctx, "", request.SnapshotID, request.Gate)
	return existing.RunID, false, existing, err
}

func (c *Coordinator) persistClosedRun(ctx context.Context, input Input, snapshot Snapshot, snapshotID, status string, local LocalEvaluation, cause error) (string, error) {
	if snapshotID == "" {
		sum := sha256.Sum256([]byte(input.ContractDID + "\x00" + input.Gate))
		snapshotID = "sha256:" + hex.EncodeToString(sum[:])
	}
	request := Request{ContractVersion: ContractVersion, CorrelationID: uuid.NewString(), SnapshotID: snapshotID, Gate: input.Gate, Snapshot: snapshot, LocalEvaluation: local}
	runID, inserted, existing, err := c.beginRun(ctx, input, request)
	if err != nil {
		return "", err
	}
	if !inserted {
		return existing.RunID, nil
	}
	return runID, c.finish(ctx, runID, status, request, nil, cause)
}

func (c *Coordinator) finish(ctx context.Context, runID, status string, request Request, response []byte, cause error) error {
	var reason any
	if cause != nil {
		reason = cause.Error()
	}
	var responseJSON any
	if len(response) > 0 {
		responseJSON = string(response)
	}
	_, err := c.DB.ExecContext(ctx, `
        UPDATE pac_workflow_gate_runs
        SET status=$2,response_json=$3::jsonb,failure_reason=$4,completed_at=CURRENT_TIMESTAMP
        WHERE run_id=$1 AND status='DISPATCHING'`,
		runID, status, responseJSON, reason)
	return err
}

func (c *Coordinator) Read(ctx context.Context, runID, snapshotID, gate string) (Run, error) {
	var run Run
	var shapes, continuation []byte
	query := `
        SELECT run_id::text,correlation_id::text,snapshot_id,contract_did,contract_version,
               contract_state,contract_updated_at,gate,status,content_hash,effective_shapes,
               profile_version,continuation_json
        FROM pac_workflow_gate_runs
        WHERE (($1 <> '' AND run_id=$1::uuid) OR ($1='' AND snapshot_id=$2 AND gate=$3))`
	err := c.DB.QueryRowxContext(ctx, query, runID, snapshotID, gate).Scan(
		&run.RunID, &run.CorrelationID, &run.SnapshotID, &run.ContractDID, &run.ContractVersion,
		&run.ContractState, &run.ContractUpdatedAt, &run.Gate, &run.Status, &run.ContentHash,
		&shapes, &run.ProfileVersion, &continuation)
	if err != nil {
		return run, err
	}
	if err := json.Unmarshal(shapes, &run.EffectiveShapes); err != nil {
		return run, fmt.Errorf("decode persisted effective shapes: %w", err)
	}
	if err := json.Unmarshal(continuation, &run.Continuation); err != nil {
		return run, fmt.Errorf("decode persisted workflow continuation: %w", err)
	}
	var decision ReviewDecision
	err = c.DB.QueryRowxContext(ctx, `
        SELECT decision_id::text,run_id::text,decision,justification,decided_by,decided_at
        FROM pac_workflow_gate_review_decisions WHERE run_id=$1
        LIMIT 1`, run.RunID).
		Scan(&decision.DecisionID, &decision.RunID, &decision.Decision, &decision.Justification, &decision.DecidedBy, &decision.DecidedAt)
	if err == nil {
		run.Review = &decision
	} else if !errors.Is(err, sql.ErrNoRows) {
		return run, err
	}
	err = c.DB.QueryRowxContext(ctx, `
        SELECT status FROM pac_workflow_gate_continuation_attempts
        WHERE run_id=$1 ORDER BY started_at DESC, attempt_id DESC LIMIT 1`, run.RunID).
		Scan(&run.ContinuationStatus)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return run, err
	}
	return run, nil
}

func (c *Coordinator) DecideReview(ctx context.Context, runID, decision, justification, decidedBy string) (ReviewDecision, error) {
	if decision != "approve" && decision != "reject" {
		return ReviewDecision{}, errors.New("review decision must be approve or reject")
	}
	if strings.TrimSpace(justification) == "" {
		return ReviewDecision{}, errors.New("review justification is required")
	}
	run, err := c.Read(ctx, runID, "", "")
	if err != nil {
		return ReviewDecision{}, err
	}
	if run.Status != "REVIEW" {
		return ReviewDecision{}, errors.New("workflow gate run is not pending review")
	}
	justification = strings.TrimSpace(justification)
	result := ReviewDecision{
		DecisionID: uuid.NewString(), RunID: runID, Decision: decision,
		Justification: justification, DecidedBy: decidedBy,
	}
	inserted := false
	if run.Review == nil {
		err = c.DB.QueryRowxContext(ctx, `
	        INSERT INTO pac_workflow_gate_review_decisions
	          (decision_id,run_id,decision,justification,decided_by)
	        VALUES ($1,$2,$3,$4,$5)
	        ON CONFLICT (run_id) DO NOTHING
	        RETURNING decision_id::text,run_id::text,decision,justification,decided_by,decided_at`,
			result.DecisionID, runID, decision, justification, decidedBy).
			Scan(&result.DecisionID, &result.RunID, &result.Decision, &result.Justification, &result.DecidedBy, &result.DecidedAt)
		switch {
		case err == nil:
			inserted = true
		case errors.Is(err, sql.ErrNoRows):
			run, err = c.Read(ctx, runID, "", "")
			if err != nil {
				return ReviewDecision{}, err
			}
			if run.Review == nil {
				return ReviewDecision{}, errors.New("concurrent workflow gate review decision is unavailable")
			}
			result = *run.Review
		default:
			return ReviewDecision{}, err
		}
	} else {
		result = *run.Review
	}
	if !inserted {
		if !sameReviewDecision(result, decision, justification, decidedBy) {
			return ReviewDecision{}, errors.New("workflow gate review decision is immutable")
		}
		if decision == "reject" || run.ContinuationStatus == "SUCCESS" {
			return result, nil
		}
		if run.ContinuationStatus == "DISPATCHING" {
			return ReviewDecision{}, errors.New("reviewed workflow continuation is already dispatching")
		}
	}
	if decision == "approve" {
		attemptID := uuid.NewString()
		if _, err := c.DB.ExecContext(ctx, `
            INSERT INTO pac_workflow_gate_continuation_attempts
              (attempt_id,run_id,decision_id,status)
	            VALUES ($1,$2,$3,'DISPATCHING')`, attemptID, runID, result.DecisionID); err != nil {
			return result, err
		}
		finishAttempt := func(status string, cause error) error {
			var reason any
			if cause != nil {
				reason = cause.Error()
			}
			_, updateErr := c.DB.ExecContext(ctx, `
                UPDATE pac_workflow_gate_continuation_attempts
                SET status=$2,failure_reason=$3,completed_at=CURRENT_TIMESTAMP
                WHERE attempt_id=$1 AND status='DISPATCHING'`,
				attemptID, status, reason)
			return updateErr
		}
		run, err = c.refreshReviewedRun(ctx, run)
		if err != nil {
			updateErr := finishAttempt("FAILED", err)
			return result, fmt.Errorf("reviewed workflow snapshot changed: %w", errors.Join(err, updateErr))
		}
		c.mu.RLock()
		resume := c.resume[run.Gate]
		c.mu.RUnlock()
		if resume == nil {
			err = fmt.Errorf("review continuation is not configured for gate %q", run.Gate)
			updateErr := finishAttempt("FAILED", err)
			return result, errors.Join(err, updateErr)
		}
		run.Review = &result
		if err := resume(ctx, run); err != nil {
			updateErr := finishAttempt("FAILED", err)
			return result, fmt.Errorf("resume reviewed workflow transition: %w", errors.Join(err, updateErr))
		}
		if err := finishAttempt("SUCCESS", nil); err != nil {
			return result, fmt.Errorf("record reviewed workflow continuation: %w", err)
		}
	}
	return result, nil
}

func sameReviewDecision(existing ReviewDecision, decision, justification, decidedBy string) bool {
	return existing.Decision == decision &&
		existing.Justification == justification &&
		existing.DecidedBy == decidedBy
}

func (c *Coordinator) refreshReviewedRun(ctx context.Context, run Run) (Run, error) {
	var version int
	var state string
	var updated time.Time
	var raw []byte
	if err := c.DB.QueryRowxContext(ctx, `
        SELECT contract_version,state,updated_at,contract_data
        FROM contracts_effective WHERE did=$1`, run.ContractDID).
		Scan(&version, &state, &updated, &raw); err != nil {
		return run, err
	}
	var content map[string]any
	if err := json.Unmarshal(raw, &content); err != nil {
		return run, fmt.Errorf("decode reviewed contract snapshot: %w", err)
	}
	return refreshReviewedSnapshot(run, version, state, updated, content)
}

func refreshReviewedSnapshot(run Run, version int, state string, updated time.Time, content map[string]any) (Run, error) {
	if version != run.ContractVersion || state != run.ContractState || hashJSON(content) != run.ContentHash {
		return run, errors.New("contract state, version, or content changed while review was pending")
	}
	// Async PDF/audit bookkeeping may advance updated_at without changing the
	// consequential contract snapshot. Continue with that current optimistic
	// concurrency token rather than the timestamp captured before human review.
	run.ContractUpdatedAt = updated
	return run, nil
}

func anchor(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case map[string]any:
		s, _ := v["@id"].(string)
		return s
	}
	return ""
}

func versionFromAnchor(value string) int {
	index := strings.LastIndex(value, "version=")
	if index < 0 {
		return 0
	}
	var version int
	_, _ = fmt.Sscanf(value[index:], "version=%d", &version)
	return version
}

func hashJSON(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}
