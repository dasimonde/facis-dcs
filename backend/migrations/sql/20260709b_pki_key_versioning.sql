-- Active HSM key version per purpose label. A key rotation inserts/advances the
-- active_version for a label; new signing operations resolve the versioned key
-- from here while historical signatures keep the version they were made with
-- (DCS-OR-C2PA-007).
CREATE TABLE pki_active_key_version (
    label          TEXT PRIMARY KEY,
    active_version INT NOT NULL,
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The HSM key version that produced a signature, so retrieval can attribute a
-- signature to the exact key version and old/new signatures stay distinguishable
-- across a rotation.
ALTER TABLE contract_signatures ADD COLUMN key_version INT NOT NULL DEFAULT 1;
