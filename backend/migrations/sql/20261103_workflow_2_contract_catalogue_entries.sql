-- DCS-IR-CWE-10, DCS-FR-CWE-23: the Federated Catalogue asset that advertises
-- a contract. A contract has at most one entry. A new registration revokes the
-- entry before it, and the entry of a contract that leaves the approved,
-- signed and active states is revoked.
CREATE TABLE contract_catalogue_entries
(
    did          VARCHAR(255) PRIMARY KEY REFERENCES contracts (did) ON DELETE CASCADE,
    asset_hash   TEXT      NOT NULL CHECK (asset_hash <> ''),
    published_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
