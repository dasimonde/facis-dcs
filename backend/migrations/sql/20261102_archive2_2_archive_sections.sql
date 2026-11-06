-- DCS-FR-CSA-26: the sections of a multi-party contract, archived one by one
-- for the parties assigned to them and linked to the archive entry of the
-- contract version. parties lists the organizations that may read the
-- section. The content, title included, lives in IPFS, encrypted under the
-- contract's key, so the erasure of the contract erases it too. Rows are
-- append-only.
CREATE TABLE contract_archive_sections (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    did              VARCHAR(255) NOT NULL,
    contract_version INT          NOT NULL,
    section_id       TEXT         NOT NULL CHECK (section_id <> ''),
    parties          TEXT[]       NOT NULL CHECK (cardinality(parties) > 0),
    content_hash     TEXT         NOT NULL CHECK (content_hash ~ '^sha256:[a-f0-9]{64}$'),
    content_cid      TEXT         NOT NULL CHECK (content_cid <> ''),
    stored_by        VARCHAR(255) NOT NULL CHECK (stored_by <> ''),
    stored_at        TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (did, contract_version) REFERENCES contract_archive_entries (did, contract_version),
    UNIQUE (did, contract_version, section_id)
);

CREATE OR REPLACE FUNCTION reject_contract_archive_section_mutation()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'contract_archive_sections is append-only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER contract_archive_sections_append_only
BEFORE UPDATE OR DELETE ON contract_archive_sections
FOR EACH ROW EXECUTE FUNCTION reject_contract_archive_section_mutation();
