-- Follow-up to 20260706_extend_contract_state_enum.sql: the new 'WITHDRAWN'
-- and 'REVOKED' enum values cannot be referenced in the same transaction
-- that adds them (Postgres restriction on ALTER TYPE ... ADD VALUE), so the
-- view updates that reference them live in this separate, later-sorted
-- migration file/transaction.
--
-- contracts_effective (and its two siblings) override a contract's stored
-- state to 'EXPIRED' once exp_date has passed, UNLESS the contract is
-- already in one of a handful of terminal states. WITHDRAWN and REVOKED are
-- both states the contract state machine treats as
-- already-final/frozen for this purpose (WITHDRAWN: initiator retracted
-- before approval; REVOKED: signature/credential invalidated post-signing)
-- — auto-expiring on top of either would be misleading, so both join the
-- existing TERMINATED/REJECTED/EXPIRED exclusion list.

CREATE OR REPLACE VIEW contracts_effective AS
SELECT
    did,
    origin,
    created_by,
    created_at,
    updated_at,
    start_date,
    exp_date,
    exp_policy,
    exp_notice_period,
    CASE
        WHEN exp_date <= CURRENT_TIMESTAMP
            AND state NOT IN ('TERMINATED', 'REJECTED', 'EXPIRED', 'WITHDRAWN', 'REVOKED')
            THEN 'EXPIRED'::contract_state
        ELSE state
        END AS state,
    contract_version,
    name,
    description,
    contract_data,
    search_vector,
    responsible,
    template_did,
    template_version
FROM contracts;

CREATE OR REPLACE VIEW contracts_effective_metadata AS
SELECT
    did,
    origin,
    created_by,
    created_at,
    updated_at,
    start_date,
    exp_date,
    exp_policy,
    exp_notice_period,
    CASE
        WHEN exp_date <= CURRENT_TIMESTAMP
            AND state NOT IN ('TERMINATED', 'REJECTED', 'EXPIRED', 'WITHDRAWN', 'REVOKED')
            THEN 'EXPIRED'::contract_state
        ELSE state
        END AS state,
    contract_version,
    name,
    description,
    responsible,
    template_did,
    template_version
FROM contracts;

CREATE OR REPLACE VIEW contracts_effective_process_data AS
SELECT
    did,
    origin,
    created_by,
    created_at,
    updated_at,
    start_date,
    exp_date,
    exp_policy,
    exp_notice_period,
    CASE
        WHEN exp_date <= CURRENT_TIMESTAMP
            AND state NOT IN ('TERMINATED', 'REJECTED', 'EXPIRED', 'WITHDRAWN', 'REVOKED')
            THEN 'EXPIRED'::contract_state
        ELSE state
        END AS state,
    contract_version
FROM contracts;
