-- Party-scoped staged counter-offers (SRS §3.1.1 Contract Negotiation UI
-- "Save draft" control): one row per (contract, party) — saved_by is the
-- participant ID, so same-party negotiators share the staged position. Rows
-- are never replicated to the peer and never enter the negotiation audit
-- trail — proposing (POST /contract/negotiate) or discarding deletes them.
CREATE TABLE IF NOT EXISTS contract_negotiation_drafts (
    contract_did VARCHAR(255) NOT NULL REFERENCES contracts (did) ON DELETE CASCADE,
    saved_by VARCHAR(255) NOT NULL,
    change_request JSONB NOT NULL,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (contract_did, saved_by)
);
