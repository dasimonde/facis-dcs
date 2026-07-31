-- outbox_events.published tracks NATS republishing independently of
-- `processed` (tamper-evident TSA/IPFS anchoring): subscribers only ever
-- consume an event's JSON payload, never an anchor-derived value, so
-- publishing must not wait behind the strictly sequential, network-bound
-- anchoring of earlier events in the same backlog.
ALTER TABLE outbox_events ADD COLUMN published BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE outbox_events ADD COLUMN published_at TIMESTAMP;

-- Events already anchored under the previous processed-then-publish
-- ordering were already republished; only genuinely pending events should
-- be republished for the first time under the new, decoupled path.
UPDATE outbox_events SET published = TRUE, published_at = processed_at WHERE processed = TRUE;

CREATE INDEX idx_outbox_events_published
    ON outbox_events(published, created_at)
    WHERE published = FALSE;
