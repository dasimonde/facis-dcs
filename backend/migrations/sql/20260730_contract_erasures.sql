-- Peer erase-handshake ledger (DCS-NFR-COMP-03, DCS-NFR-SEC-13): one row per
-- (contract, counterparty instance) erase request. A row without confirmed_at
-- is pending and drives the retry scheduler (same mechanics as sync_fails,
-- kept separate so PDF ships and erasures never mix); confirmed_at records
-- when the peer acknowledged shredding its wrapped CEKs.
CREATE TABLE contract_erasures
(
    id           BIGSERIAL PRIMARY KEY,
    did          VARCHAR(255) NOT NULL,
    peer_did     VARCHAR(255) NOT NULL,
    requested_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    retry_count  INT          NOT NULL DEFAULT 0,
    last_tried_at TIMESTAMPTZ,
    confirmed_at TIMESTAMPTZ,
    UNIQUE (did, peer_did)
);
