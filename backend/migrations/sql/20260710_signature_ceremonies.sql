CREATE TABLE signature_ceremonies (
    id             UUID PRIMARY KEY,
    contract_did   VARCHAR(255) NOT NULL REFERENCES contracts(did),
    field_name     VARCHAR(255) NOT NULL,
    requested_by   VARCHAR(255) NOT NULL,
    status         VARCHAR(32)  NOT NULL DEFAULT 'pending',
    wallet_uri     TEXT,
    nonce          VARCHAR(255) NOT NULL,
    signer_did     VARCHAR(255),
    vp_token       TEXT,
    pid_claims     JSONB,
    kb_sd_hash     VARCHAR(255),
    created_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    verified_at    TIMESTAMP,
    expires_at     TIMESTAMP NOT NULL
);

CREATE INDEX idx_signature_ceremonies_contract ON signature_ceremonies(contract_did);
CREATE INDEX idx_signature_ceremonies_status   ON signature_ceremonies(status);

ALTER TABLE contract_signatures ADD COLUMN ceremony_id  UUID REFERENCES signature_ceremonies(id);
ALTER TABLE contract_signatures ADD COLUMN pdf_hash     VARCHAR(255);
ALTER TABLE contract_signatures ADD COLUMN content_hash VARCHAR(255);
