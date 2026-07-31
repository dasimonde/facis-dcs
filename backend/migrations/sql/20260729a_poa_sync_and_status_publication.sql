-- Transferable, receiver-revalidated PoA evidence is part of the existing
-- cross-instance signing provenance, not a new credential type.
ALTER TABLE contract_sync_signatures
    ADD COLUMN poa_evidence JSONB,
    ADD COLUMN poa_revalidated_at TIMESTAMP;

-- Durable, idempotent intent queue for XFSC contract-status publication.
-- Lifecycle commands enqueue in their own transaction; the immediate worker
-- retries failed entries until the service acknowledges the deterministic bit.
CREATE TABLE status_publication_queue
(
    contract_did    VARCHAR(255) NOT NULL,
    status          VARCHAR(32)  NOT NULL,
    reason          TEXT         NOT NULL DEFAULT '',
    effective_at    TIMESTAMP    NOT NULL,
    created_at      TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    next_attempt_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    attempt_count   INT          NOT NULL DEFAULT 0,
    last_error      TEXT,
    published_at    TIMESTAMP,
    PRIMARY KEY (contract_did, status)
);

CREATE INDEX status_publication_queue_pending_idx
    ON status_publication_queue (next_attempt_at, created_at)
    WHERE published_at IS NULL;
