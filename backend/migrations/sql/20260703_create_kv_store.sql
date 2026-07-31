CREATE TABLE IF NOT EXISTS kv_store
(
    key        TEXT        PRIMARY KEY,
    value      TEXT        NOT NULL,
    expires_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_kv_store_expires_at
    ON kv_store (expires_at)
    WHERE expires_at IS NOT NULL;
