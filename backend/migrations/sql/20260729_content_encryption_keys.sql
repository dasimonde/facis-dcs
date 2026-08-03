-- Wrapped content-encryption keys for IPFS artifacts (DCS-NFR-SEC-14).
-- One CEK per (scope, recipient), wrapped to the recipient's keyAgreement key.
-- Shredding marks rows destroyed (DCS-NFR-SEC-13, DCS-NFR-COMP-03); the row
-- itself is the destruction record and is never deleted.
CREATE TABLE content_encryption_keys (
    scope_kind    VARCHAR(16) NOT NULL,
    scope_id      TEXT        NOT NULL,
    recipient_did TEXT        NOT NULL,
    wrapped_cek   JSONB       NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    shredded_at   TIMESTAMPTZ,
    shredded_by   TEXT,
    shred_reason  TEXT
);

CREATE UNIQUE INDEX content_encryption_keys_live_unique
    ON content_encryption_keys (scope_kind, scope_id, recipient_did)
    WHERE shredded_at IS NULL;
