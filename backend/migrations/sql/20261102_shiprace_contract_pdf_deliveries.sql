-- The contract PDF revision each counterparty peer last accepted, or refused
-- as a signing conflict (ADR-13, ADR-38).
-- One row per (contract, peer): the contract state and the stored PDF's CID
-- that were shipped. A ship attempt skips a peer that already holds the
-- current revision, so concurrent or repeated triggers for one revision post
-- it to the peer once.
CREATE TABLE IF NOT EXISTS contract_pdf_deliveries
(
    did            VARCHAR(255) NOT NULL,
    peer_did       VARCHAR(255) NOT NULL,
    contract_state VARCHAR(64)  NOT NULL,
    pdf_cid        TEXT         NOT NULL,
    delivered_at   TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (did, peer_did)
);
