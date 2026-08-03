-- A party may not pass a mutual milestone on local state alone: signing a
-- contract claims both parties settled the same version, so the counterparty's
-- settlement must be held locally as verified evidence rather than inferred
-- from the absence of an objection (ADR-13 keeps intrinsic state local, which
-- is exactly why the peer's own state is not observable here).
--
-- One row per (contract, settling party, audience). A row whose from_peer_did
-- is this instance's own did:web is a settlement it produced: delivered_at
-- stays NULL until the peer accepted the ship, so the retry scheduler can
-- re-deliver it. A row from a counterparty is the evidence the signing gate
-- reads; it is stored only after the JAdES verified against that peer's
-- published assertion key.
--
-- Kept apart from contract_sync_signatures deliberately: that table is keyed by
-- contract alone with a NOT NULL signature over the contract document, one row
-- per contract, and it answers a later question ("the peer signed"). A
-- settlement exists before any signature does, is per party, and binds the
-- document by digest.
CREATE TABLE IF NOT EXISTS contract_settlements
(
    did              VARCHAR(255) NOT NULL CHECK (did <> ''),
    from_peer_did    VARCHAR(255) NOT NULL CHECK (from_peer_did <> ''),
    to_peer_did      VARCHAR(255) NOT NULL CHECK (to_peer_did <> ''),
    contract_version INT          NOT NULL,
    -- The JCS-canonicalized contract document's sha256, prefixed "sha256:".
    -- This is the version identity that binds across instances; the integer
    -- contract_version above is a per-instance counter and is recorded for
    -- diagnostics only, never compared against the local one.
    document_digest  VARCHAR(80)  NOT NULL CHECK (document_digest <> ''),
    settled_at       TIMESTAMP    NOT NULL,
    jades_signature  TEXT         NOT NULL CHECK (jades_signature <> ''),
    recorded_at      TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    delivered_at     TIMESTAMP,
    PRIMARY KEY (did, from_peer_did, to_peer_did)
);
