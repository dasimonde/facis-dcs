-- DCS-FR-SM-07/-17 (multi-signer): each signature records WHICH declared
-- signature field it covers, so the deploy gate can check that every
-- signatureFields entry of the contract document is signed before the
-- contract may activate, and a field can never be signed twice.
ALTER TABLE contract_signatures
    ADD COLUMN field_name VARCHAR(255);

CREATE INDEX idx_contract_signatures_field
    ON contract_signatures (contract_did, field_name);
