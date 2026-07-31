CREATE TABLE IF NOT EXISTS trusted_peers
(
    peer_did VARCHAR(255) PRIMARY KEY
);

CREATE TABLE IF NOT EXISTS sync_fails
(
    id          BIGSERIAL PRIMARY KEY,
    did         VARCHAR(255) NOT NULL UNIQUE,
    retry_count INT          NOT NULL DEFAULT 0,
    created_at  TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_tried_at TIMESTAMP
);