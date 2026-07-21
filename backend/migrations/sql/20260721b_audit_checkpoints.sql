-- Global tamper evidence moves from a per-event chain link to Merkle
-- checkpoints (see base/datatype.AuditCheckpoint). The per-event global link
-- serialized the whole outbox behind one TSA + IPFS round-trip per event and
-- let a single failing event stall every event behind it; nothing ever read it
-- back. A checkpoint commits to a whole batch with one root, one TSA timestamp
-- and a link to the previous root, which is the same append-only guarantee at
-- a fraction of the cost.

CREATE TABLE IF NOT EXISTS audit_checkpoints (
    seq            BIGSERIAL PRIMARY KEY,
    cid            VARCHAR(120) NOT NULL,
    root           VARCHAR(64)  NOT NULL,
    prev_root      VARCHAR(64),
    leaf_count     INTEGER      NOT NULL,
    -- NULL while the TSA has not answered yet: the root is immutable, so the
    -- timestamp can be attached later without rewriting the checkpoint.
    tsa_signature  TEXT,
    created_at     TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    timestamped_at TIMESTAMP
);

CREATE INDEX IF NOT EXISTS audit_checkpoints_pending_timestamp
    ON audit_checkpoints (seq) WHERE tsa_signature IS NULL;

-- The global head is a checkpoint root now, not a row in the per-resource CID
-- table; the per-resource chains keep using audit_trail_log unchanged.
DELETE FROM audit_trail_log WHERE component = 'GLOBAL_AUDIT_TRAIL' AND did = 'GLOBAL_AUDIT_TRAIL';

-- Leaf index: which checkpoint an anchored entry sits in, and at which
-- position. Needed to serve an inclusion proof for one entry without reading
-- the whole batch back out of IPFS. The leaf hash is blinded by the entry's
-- nonce (see datatype.AuditLogEntry.Nonce), so these rows commit to the
-- entries without revealing anything about them.
CREATE TABLE IF NOT EXISTS audit_checkpoint_leaves (
    checkpoint_seq BIGINT       NOT NULL REFERENCES audit_checkpoints (seq) ON DELETE CASCADE,
    idx            INTEGER      NOT NULL,
    entry_cid      VARCHAR(120) NOT NULL,
    leaf_hash      VARCHAR(64)  NOT NULL,
    PRIMARY KEY (checkpoint_seq, idx)
);

CREATE INDEX IF NOT EXISTS audit_checkpoint_leaves_entry_cid
    ON audit_checkpoint_leaves (entry_cid);
