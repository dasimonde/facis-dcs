-- DCS-FR-SM-02 (JAdES): every DCS-to-DCS contract broadcast is signed by the
-- origin peer as a JAdES baseline-B compact JWS over the canonical contract
-- representation (internal/base/jades). The receiving instance verifies the
-- signature and its binding to the sender's did:web key before accepting the
-- sync, then persists the artifact here so the contract's cross-instance
-- provenance stays independently verifiable (exposed via
-- GET /peer/contracts/provenance).
CREATE TABLE IF NOT EXISTS contract_sync_signatures
(
    did              VARCHAR(255) PRIMARY KEY CHECK (did <> ''),
    contract_version INT          NOT NULL,
    from_peer_did    VARCHAR(255) NOT NULL CHECK (from_peer_did <> ''),
    jades_signature  TEXT         NOT NULL CHECK (jades_signature <> ''),
    received_at      TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
);
