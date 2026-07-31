-- The @clean_db test teardown deletes the contract-child tables before
-- `contracts`, but the backend's async writers (KPI ingestion from the target
-- callback, deployment records, signing ceremonies) can insert a child row into
-- the window between the child wipe and the `contracts` wipe, so the subsequent
-- `DELETE FROM contracts` trips the child FK ("still referenced from table
-- contract_kpis"). ON DELETE CASCADE makes deleting a contract remove its
-- children atomically, closing the race. Contracts are state-transitioned,
-- never row-deleted, in normal operation, so this only affects test cleanup.
ALTER TABLE contract_kpis
  DROP CONSTRAINT contract_kpis_did_fkey,
  ADD CONSTRAINT contract_kpis_did_fkey
    FOREIGN KEY (did) REFERENCES contracts (did) ON DELETE CASCADE;

ALTER TABLE contract_deployments
  DROP CONSTRAINT contract_deployments_did_fkey,
  ADD CONSTRAINT contract_deployments_did_fkey
    FOREIGN KEY (did) REFERENCES contracts (did) ON DELETE CASCADE;

ALTER TABLE signature_ceremonies
  DROP CONSTRAINT signature_ceremonies_contract_did_fkey,
  ADD CONSTRAINT signature_ceremonies_contract_did_fkey
    FOREIGN KEY (contract_did) REFERENCES contracts (did) ON DELETE CASCADE;
