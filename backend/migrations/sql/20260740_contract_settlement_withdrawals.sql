-- Withdrawing a settlement is a statement about both parties, exactly as
-- making one is: the counterparty holds this instance's settlement as the
-- evidence its signing gate reads, and deleting the local row (the workflow
-- engine's rejection edges) tells that peer nothing. Until the withdrawal
-- lands, the peer may sign a version this instance no longer agrees to.
--
-- The outbound queue that carries it is shaped like contract_settlements': one
-- row per (contract, withdrawing party, audience), delivered_at NULL until the
-- peer accepted it, drained by the same retry scheduler. A later withdrawal
-- toward the same peer replaces an undelivered earlier one — it names the
-- later settlement, which is the one that has to go.
--
-- document_digest is the version the withdrawn settlement covered, not the
-- document as it now stands. It is what makes a withdrawal unreplayable: the
-- receiver removes its held settlement only when that settlement names this
-- same version, so a withdrawal held back and re-delivered into a later round
-- matches nothing and removes nothing.
--
-- No JAdES column: the artifact is signed at delivery from these fields, so a
-- retry after a key rotation still ships a signature the peer can verify.
-- withdrawn_at is stored, and signed as stored, so every attempt carries the
-- same statement.
CREATE TABLE IF NOT EXISTS contract_settlement_withdrawals
(
    did             VARCHAR(255) NOT NULL CHECK (did <> ''),
    from_peer_did   VARCHAR(255) NOT NULL CHECK (from_peer_did <> ''),
    to_peer_did     VARCHAR(255) NOT NULL CHECK (to_peer_did <> ''),
    document_digest VARCHAR(80)  NOT NULL CHECK (document_digest <> ''),
    withdrawn_at    TIMESTAMP    NOT NULL,
    recorded_at     TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    delivered_at    TIMESTAMP,
    PRIMARY KEY (did, from_peer_did, to_peer_did)
);
