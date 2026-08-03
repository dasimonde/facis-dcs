CREATE TABLE pac_workflow_gate_runs (
    run_id              UUID PRIMARY KEY,
    correlation_id      UUID NOT NULL UNIQUE,
    snapshot_id         TEXT NOT NULL,
    contract_did        TEXT NOT NULL,
    contract_version    INTEGER NOT NULL,
    contract_state      TEXT NOT NULL,
    contract_updated_at TIMESTAMPTZ NOT NULL,
    gate                TEXT NOT NULL,
    status              TEXT NOT NULL CHECK (status IN ('DISPATCHING','SUCCESS','REVIEW','BLOCKED')),
    content_hash        TEXT NOT NULL,
    snapshot_json       JSONB NOT NULL,
    effective_shapes    JSONB NOT NULL,
    profile_version     INTEGER NOT NULL,
    local_evaluation    JSONB NOT NULL,
    continuation_json   JSONB NOT NULL DEFAULT '{}'::jsonb,
    request_json        JSONB,
    response_json       JSONB,
    failure_reason      TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at        TIMESTAMPTZ,
    UNIQUE (snapshot_id, gate)
);

CREATE INDEX idx_pac_workflow_gate_runs_contract
    ON pac_workflow_gate_runs (contract_did, contract_version, gate);

CREATE TABLE pac_workflow_gate_review_decisions (
    decision_id    UUID PRIMARY KEY,
    run_id         UUID NOT NULL UNIQUE REFERENCES pac_workflow_gate_runs(run_id),
    decision       TEXT NOT NULL CHECK (decision IN ('approve','reject')),
    justification  TEXT NOT NULL CHECK (length(trim(justification)) > 0),
    decided_by     TEXT NOT NULL,
    decided_at     TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE pac_workflow_gate_continuation_attempts (
    attempt_id     UUID PRIMARY KEY,
    run_id         UUID NOT NULL REFERENCES pac_workflow_gate_runs(run_id),
    decision_id    UUID NOT NULL REFERENCES pac_workflow_gate_review_decisions(decision_id),
    status         TEXT NOT NULL CHECK (status IN ('DISPATCHING','SUCCESS','FAILED')),
    failure_reason TEXT,
    started_at     TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at   TIMESTAMPTZ
);

CREATE INDEX idx_pac_workflow_gate_continuation_attempts_run
    ON pac_workflow_gate_continuation_attempts (run_id, started_at DESC);
CREATE UNIQUE INDEX uq_pac_workflow_gate_continuation_dispatch
    ON pac_workflow_gate_continuation_attempts (run_id)
    WHERE status = 'DISPATCHING';

COMMENT ON TABLE pac_workflow_gate_runs IS
    'Stable two-phase workflow gate executions, unique per immutable snapshot and gate.';
COMMENT ON TABLE pac_workflow_gate_review_decisions IS
    'Append-only persistent Compliance Officer decisions for workflow gate REVIEW results.';
COMMENT ON TABLE pac_workflow_gate_continuation_attempts IS
    'Append-only retry history for delayed manual-review workflow continuations.';
