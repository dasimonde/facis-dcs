-- API request budget per credential (DCS-FR-CWE-28), shared by all replicas
-- of one DCS instance. One row per credential: the start of its current
-- one-minute window and the requests counted in it. credential_hash is a
-- truncated SHA-256 of the Authorization header; raw tokens are not stored.
-- UNLOGGED: the counters are short-lived and need no crash safety.
CREATE UNLOGGED TABLE IF NOT EXISTS api_rate_limit_windows
(
    credential_hash BYTEA       PRIMARY KEY,
    window_start    TIMESTAMPTZ NOT NULL,
    request_count   INTEGER     NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_api_rate_limit_windows_start ON api_rate_limit_windows (window_start);
