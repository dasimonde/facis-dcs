-- ADR-27: every machine caller authenticates with its own OAuth2 client
-- provisioned in Hydra, instead of a secret held in deployment configuration.
--
-- Two things were wrong with what this replaces. System users carried their
-- secret in plaintext in a values file, so adding an integrator or rotating a
-- credential meant editing YAML and redeploying. Contract target systems shared
-- ONE secret (DEPLOYMENT_CALLBACK_SECRET) across every target, so a callback
-- could only prove that some target was calling, not which, and one compromised
-- target compromised all of them.
--
-- Hydra stores the secret hashed and cannot return it again, which is what lets
-- the credential be shown exactly once and rotated rather than recovered.

CREATE TABLE machine_identities (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name            VARCHAR(255) NOT NULL UNIQUE,
    -- The OAuth2 client Hydra holds. The access token's client_id claim is
    -- matched against this to resolve what the caller may do.
    oauth_client_id VARCHAR(255) NOT NULL UNIQUE,
    -- Attributed identity in the audit trail, standing in for the ext.iss a
    -- human token carries.
    participant_did VARCHAR(255) NOT NULL,
    -- JSON array of DCS role strings. Authority is read from here by client_id
    -- and never from the token, so a caller cannot widen its own reach.
    roles           TEXT         NOT NULL,
    description     TEXT,
    enabled         BOOLEAN      NOT NULL DEFAULT TRUE,
    -- When the current secret was issued. The secret itself is never stored;
    -- this only lets an operator see how old a credential is.
    secret_issued_at TIMESTAMP,
    created_by      VARCHAR(255) NOT NULL,
    created_at      TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_machine_identities_client_id ON machine_identities(oauth_client_id);

-- A contract target authenticates as itself rather than with the shared secret,
-- so its callback proves which target is reporting. Nullable: an entry seeded
-- before its client exists is still a readable destination, and dispatch does
-- not depend on the target having a credential.
ALTER TABLE contract_targets
    ADD COLUMN oauth_client_id VARCHAR(255) UNIQUE,
    ADD COLUMN secret_issued_at TIMESTAMP;
