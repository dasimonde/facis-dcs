-- Persist every successful externally executed PAC audit as immutable evidence.
CREATE TABLE pac_audit_runs (
    audit_id          uuid PRIMARY KEY,
    correlation_id    uuid NOT NULL UNIQUE,
    contract_version  TEXT NOT NULL,
    scope             TEXT NOT NULL,
    resource_did      TEXT,
    requester         TEXT NOT NULL,
    justification     TEXT NOT NULL,
    request_hash      TEXT NOT NULL,
    response_hash     TEXT NOT NULL,
    executor_id       TEXT NOT NULL,
    executor_version  TEXT NOT NULL,
    executed_at       TIMESTAMPTZ NOT NULL,
    receipt           JSONB,
    response_json     JSONB NOT NULL,
    response_bytes    BYTEA NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_pac_audit_runs_report
    ON pac_audit_runs (scope, resource_did, created_at DESC);

COMMENT ON TABLE pac_audit_runs IS
    'Append-only successful results from the configured PAC audit executor.';

CREATE OR REPLACE FUNCTION reject_pac_audit_run_mutation()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'pac_audit_runs is append-only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER pac_audit_runs_append_only
BEFORE UPDATE OR DELETE ON pac_audit_runs
FOR EACH ROW EXECUTE FUNCTION reject_pac_audit_run_mutation();
