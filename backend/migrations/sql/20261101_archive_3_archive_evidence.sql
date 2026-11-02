-- DCS-IR-CSA-02: evidence documents the Archive Manager stores for an
-- archived contract. The content lives in IPFS, encrypted under the
-- contract's key, so the erasure of the contract erases it too. Rows are
-- append-only.
CREATE TABLE contract_archive_evidence (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    did          VARCHAR(255) NOT NULL REFERENCES contracts (did),
    name         TEXT         NOT NULL CHECK (name <> ''),
    media_type   TEXT         NOT NULL CHECK (media_type <> ''),
    description  TEXT,
    content_hash TEXT         NOT NULL CHECK (content_hash ~ '^sha256:[a-f0-9]{64}$'),
    content_cid  TEXT         NOT NULL CHECK (content_cid <> ''),
    size_bytes   BIGINT       NOT NULL,
    stored_by    VARCHAR(255) NOT NULL CHECK (stored_by <> ''),
    stored_at    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_contract_archive_evidence_did ON contract_archive_evidence (did);

CREATE OR REPLACE FUNCTION reject_contract_archive_evidence_mutation()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'contract_archive_evidence is append-only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER contract_archive_evidence_append_only
BEFORE UPDATE OR DELETE ON contract_archive_evidence
FOR EACH ROW EXECUTE FUNCTION reject_contract_archive_evidence_mutation();
