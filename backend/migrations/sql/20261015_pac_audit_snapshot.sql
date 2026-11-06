-- Pre-release schema: existing runs cannot recover the unpersisted request evidence.
ALTER TABLE pac_audit_runs
    ADD COLUMN snapshot_bytes BYTEA NOT NULL,
    DROP COLUMN response_bytes,
    DROP COLUMN response_json,
    DROP COLUMN receipt;

COMMENT ON COLUMN pac_audit_runs.snapshot_bytes IS
    'Authenticated encrypted request/response snapshot, dependent on every private evidence resource CEK.';
