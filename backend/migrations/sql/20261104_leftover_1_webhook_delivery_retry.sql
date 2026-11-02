-- One row per delivery of a dispatched event to one subscription, written
-- before the first attempt (DCS-IR-SI-02). A delivery is tried again at
-- next_attempt_at until the subscriber answers 2xx or the attempts run out;
-- next_attempt_at and payload are NULL once it ends. A callback acknowledges
-- the deliveries of the caller's organization only.
ALTER TABLE webhook_deliveries
    ADD COLUMN subscription_id UUID,
    ADD COLUMN payload         BYTEA,
    ADD COLUMN attempts        INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN next_attempt_at TIMESTAMPTZ,
    ADD COLUMN acknowledged_at TIMESTAMPTZ;

ALTER TABLE webhook_deliveries ALTER COLUMN delivered_at DROP NOT NULL;

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_due
    ON webhook_deliveries (next_attempt_at) WHERE next_attempt_at IS NOT NULL;

ALTER TABLE webhook_dispatches DROP COLUMN acknowledged_at;
