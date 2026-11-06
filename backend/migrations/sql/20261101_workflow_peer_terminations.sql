-- Peer termination ledger (DCS-NFR-BR-06): one row per (contract, counterparty
-- instance) termination notice. The terminating instance writes the row in the
-- same transaction as the state change. A row without confirmed_at is pending
-- and is re-delivered until the peer acknowledges it.
CREATE TABLE contract_peer_terminations
(
    id            BIGSERIAL PRIMARY KEY,
    did           VARCHAR(255) NOT NULL,
    peer_did      VARCHAR(255) NOT NULL,
    reason        TEXT         NOT NULL DEFAULT '',
    requested_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    retry_count   INT          NOT NULL DEFAULT 0,
    last_tried_at TIMESTAMPTZ,
    last_error    TEXT,
    confirmed_at  TIMESTAMPTZ,
    UNIQUE (did, peer_did)
);
