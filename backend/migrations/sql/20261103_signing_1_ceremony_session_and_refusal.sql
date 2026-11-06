-- A ceremony belongs to the session that started it: its organization
-- (requested_by) and its holder. The last refused wallet response is kept on
-- the ceremony so that the signer sees it, and it bounds how often a refusal
-- is audited. document_retrieved_sha256 is the to-be-signed document whose
-- retrieval by the wallet is audited.
ALTER TABLE signature_ceremonies
    ADD COLUMN requested_holder_did      VARCHAR(255) NOT NULL DEFAULT '',
    ADD COLUMN last_refusal_code         VARCHAR(64),
    ADD COLUMN last_refusal              TEXT,
    ADD COLUMN last_refusal_at           TIMESTAMP,
    ADD COLUMN document_retrieved_sha256 VARCHAR(64);
