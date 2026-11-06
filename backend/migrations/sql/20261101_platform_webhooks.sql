-- Webhook dispatch state shared by all replicas of one DCS instance
-- (DCS-NFR-PER-02). The replica that dispatches an event and the replica that
-- receives the subscriber callback are not the same replica.

-- One row per dispatched event. acknowledged_at is set by the first callback
-- for the event_id; a later callback finds no pending row.
CREATE TABLE IF NOT EXISTS webhook_dispatches
(
    event_id        VARCHAR(255) PRIMARY KEY,
    correlation_id  VARCHAR(255) NOT NULL,
    event           VARCHAR(255) NOT NULL,
    did             TEXT         NOT NULL,
    sent_at         TIMESTAMPTZ  NOT NULL,
    acknowledged_at TIMESTAMPTZ
);

-- One row per notification attempt to one subscriber. owner_did is the
-- organization that owns the subscription; GET /deliveries is scoped to it.
CREATE TABLE IF NOT EXISTS webhook_deliveries
(
    id           BIGSERIAL PRIMARY KEY,
    event_id     VARCHAR(255) NOT NULL REFERENCES webhook_dispatches (event_id) ON DELETE CASCADE,
    callback_url TEXT         NOT NULL,
    owner_did    VARCHAR(255) NOT NULL,
    status_code  INTEGER      NOT NULL DEFAULT 0,
    error        TEXT         NOT NULL DEFAULT '',
    delivered_at TIMESTAMPTZ  NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_owner_did ON webhook_deliveries (owner_did, id);
