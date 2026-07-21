-- An audit event that can never be anchored (malformed payload, a CID the
-- store will not resolve) used to be retried on every tick, silently, forever.
-- It no longer blocks the batch, but it must not disappear either: count the
-- attempts, keep the last error, and once the count is exhausted mark the event
-- dead-lettered so the anchoring loop stops picking it up and an operator can
-- find it (SELECT ... WHERE dead_lettered_at IS NOT NULL).

ALTER TABLE outbox_events
    ADD COLUMN IF NOT EXISTS anchor_attempts  INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS anchor_error     TEXT,
    ADD COLUMN IF NOT EXISTS dead_lettered_at TIMESTAMP;

CREATE INDEX IF NOT EXISTS outbox_events_dead_lettered
    ON outbox_events (dead_lettered_at) WHERE dead_lettered_at IS NOT NULL;
