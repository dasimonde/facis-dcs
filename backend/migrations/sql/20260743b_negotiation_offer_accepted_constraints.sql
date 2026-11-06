-- Follow-up to 20260743_negotiation_offer_accepted.sql: CHECK and unique index
-- that reference OFFER_ACCEPTED cannot share the ADD VALUE transaction
-- (Postgres 55P04). OFFER_ACCEPTED carries no domain negotiation row;
-- negotiation_id is the zero UUID sentinel on the wire and in this queue.

ALTER TABLE contract_negotiation_outbound
    DROP CONSTRAINT IF EXISTS chk_negotiation_outbound_decision_for_kind;

ALTER TABLE contract_negotiation_outbound
    ADD CONSTRAINT chk_negotiation_outbound_decision_for_kind CHECK (
        (kind IN ('PROPOSAL', 'WITHDRAW') AND decision_id IS NULL)
        OR (kind IN ('ACCEPT', 'REJECT') AND decision_id IS NOT NULL)
        OR (kind = 'OFFER_ACCEPTED' AND decision_id IS NULL)
    );

CREATE UNIQUE INDEX IF NOT EXISTS uq_negotiation_outbound_offer_accepted
    ON contract_negotiation_outbound (did, to_peer_did)
    WHERE kind = 'OFFER_ACCEPTED';
