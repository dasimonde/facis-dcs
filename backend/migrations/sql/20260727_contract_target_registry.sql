-- ADR-25: contract target systems become a configured registry, and each
-- contract designates the one it deploys to.
--
-- Deployment previously addressed a single endpoint held in CONTRACT_TARGET_URL
-- and read at dispatch time, so a DCS serving several execution environments
-- could not express where a contract should go (DCS-IR-SI-05 specifies the
-- interface against target systemS), and a failure had no target to name.

CREATE TABLE contract_targets (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(255) NOT NULL UNIQUE,
    url         TEXT         NOT NULL,
    description TEXT,
    -- Disabled entries stay referenceable so a contract that already names one
    -- keeps a readable destination; dispatch to a disabled target is refused.
    enabled     BOOLEAN      NOT NULL DEFAULT TRUE,
    created_by  VARCHAR(255) NOT NULL,
    created_at  TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- The contract's own deployment destination (DCS-FR-SM-12's automatic trigger
-- has no human present to choose one). ON DELETE RESTRICT: removing a registry
-- entry a contract still names would leave that contract undeployable with no
-- record of where it was meant to go — the admin must repoint it first.
ALTER TABLE contracts
    ADD COLUMN target_id uuid REFERENCES contract_targets(id) ON DELETE RESTRICT;

CREATE INDEX idx_contracts_target_id ON contracts(target_id);

-- A deployment records WHICH registry entry it went to, and separately the
-- endpoint as it stood at dispatch: editing an entry's URL later must not
-- rewrite what an earlier deployment actually did.
ALTER TABLE contract_deployments
    ADD COLUMN target_id uuid REFERENCES contract_targets(id) ON DELETE SET NULL;

-- Rows were written 'DISPATCHED' BEFORE the outbound call was attempted and a
-- failed call was only written to the process log, so a deployment the target
-- never received was indistinguishable from one it acknowledged. Status
-- 'DISPATCH_FAILED' marks those, and dispatch_error records why, so the
-- compliance monitor can raise an alert naming the reason (DCS-FR-CWE-31).
ALTER TABLE contract_deployments
    ADD COLUMN dispatch_error TEXT;

CREATE INDEX idx_contract_deployments_status ON contract_deployments(status);

-- contracts_effective is deliberately NOT rebuilt to carry target_id.
-- contracts_archive_metadata is defined on top of it, so dropping it needs a
-- CASCADE and a faithful re-creation of a view that has evolved across five
-- migrations — a large risk for one column. The contract read joins contracts
-- for the designation instead (see PostgresContractRepo.ReadDataByDID).
