ALTER TABLE contracts
    ADD COLUMN IF NOT EXISTS pdf_payload_hash TEXT;

ALTER TABLE contract_templates
    ADD COLUMN IF NOT EXISTS pdf_payload_hash TEXT;
