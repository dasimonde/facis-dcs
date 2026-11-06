-- DCS-FR-SM-10: the proof of contract execution over every signature. When
-- the last counterparty signature arrives after this instance archived the
-- contract, a proof over the archived signatures, the counterparty signatures
-- and the fully signed PDF is notarized, timestamped and kept here. Rows are
-- append-only.
CREATE TABLE contract_execution_proofs (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    did            VARCHAR(255) NOT NULL REFERENCES contracts (did),
    content_hash   TEXT         NOT NULL CHECK (content_hash ~ '^sha256:[a-f0-9]{64}$'),
    proof          JSONB        NOT NULL,
    notary_receipt JSONB,
    tsa_receipt    JSONB,
    created_at     TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (did, content_hash)
);

CREATE OR REPLACE FUNCTION reject_contract_execution_proof_mutation()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'contract_execution_proofs is append-only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER contract_execution_proofs_append_only
BEFORE UPDATE OR DELETE ON contract_execution_proofs
FOR EACH ROW EXECUTE FUNCTION reject_contract_execution_proof_mutation();
