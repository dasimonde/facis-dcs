-- Outbound queue for negotiation messages (Proposals, Accept, Reject,
-- Withdraw) that must reach a peer DCS. Domain rows in contract_negotiations /
-- contract_negotiation_decisions are created locally first; this table records
-- each pending ship so peer downtime does not fail that local create, and so a
-- restart can retry undelivered messages.
--
-- id is the stable message identity on the wire. Retries re-send the same id so
-- the receiver can upsert idempotently. delivered_at stays NULL until the peer
-- acknowledges durable persistence (received-and-stored, not accepted as a
-- contract change).
--
-- kind + negotiation_id (+ decision_id for Accept/Reject) point at the domain
-- fact being shipped. A Counteroffer is another PROPOSAL. WITHDRAW retracts a
-- Proposal the author still owns before the counterparty has accepted it. The
-- ship reads the domain tables at delivery time for PROPOSAL/ACCEPT/REJECT; a
-- WITHDRAW only needs negotiation_id (the domain row is already deleted).
--
-- Partial unique indexes keep one outbound row per (message fact, audience).
-- PROPOSAL/WITHDRAW rows leave decision_id NULL, so a plain UNIQUE including
-- decision_id would not collapse them under PostgreSQL NULL semantics.

CREATE TYPE contract_negotiation_message_kind AS ENUM (
    'PROPOSAL',
    'ACCEPT',
    'REJECT',
    'WITHDRAW'
);

CREATE TABLE IF NOT EXISTS contract_negotiation_outbound
(
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    did              VARCHAR(255) NOT NULL CHECK (did <> ''),
    from_peer_did    VARCHAR(255) NOT NULL CHECK (from_peer_did <> ''),
    to_peer_did      VARCHAR(255) NOT NULL CHECK (to_peer_did <> ''),

    kind             contract_negotiation_message_kind NOT NULL,
    negotiation_id   uuid NOT NULL,
    decision_id      uuid,
    contract_version INT  NOT NULL,

    recorded_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    delivered_at     TIMESTAMP,

    CONSTRAINT chk_negotiation_outbound_decision_for_kind CHECK (
        (kind IN ('PROPOSAL', 'WITHDRAW') AND decision_id IS NULL)
        OR (kind IN ('ACCEPT', 'REJECT') AND decision_id IS NOT NULL)
    )
);

CREATE UNIQUE INDEX uq_negotiation_outbound_proposal
    ON contract_negotiation_outbound (negotiation_id, to_peer_did)
    WHERE kind = 'PROPOSAL';

CREATE UNIQUE INDEX uq_negotiation_outbound_withdraw
    ON contract_negotiation_outbound (negotiation_id, to_peer_did)
    WHERE kind = 'WITHDRAW';

CREATE UNIQUE INDEX uq_negotiation_outbound_decision
    ON contract_negotiation_outbound (decision_id, to_peer_did)
    WHERE kind IN ('ACCEPT', 'REJECT') AND decision_id IS NOT NULL;

CREATE INDEX idx_negotiation_outbound_undelivered
    ON contract_negotiation_outbound (from_peer_did, recorded_at)
    WHERE delivered_at IS NULL;

CREATE INDEX idx_negotiation_outbound_did
    ON contract_negotiation_outbound (did);

-- Tombstone for a withdrawn proposal. Survives the domain DELETE so a late
-- Accept/Reject (or a re-delivered Proposal) that arrives after a partition
-- can ACK as a no-op instead of being confused with "Proposal not held yet,
-- retry". Without this, Accept-before-Proposal ordering and Withdraw-vs-Accept
-- races cannot be told apart.
CREATE TABLE IF NOT EXISTS contract_negotiation_withdrawn
(
    negotiation_id uuid PRIMARY KEY,
    did            VARCHAR(255) NOT NULL CHECK (did <> ''),
    withdrawn_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_negotiation_withdrawn_did
    ON contract_negotiation_withdrawn (did);
