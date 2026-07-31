-- The negotiation optimistic-lock guard compared the caller's fetched timestamp
-- against contracts.updated_at, which moves on ANY write to the row (a state
-- transition, a background artifact write, etc.) — not only a real content edit.
-- A benign concurrent bump therefore false-tripped the lost-update guard
-- ("contract was updated elsewhere") even though the contract's content had not
-- changed. Track a separate content_updated_at that moves ONLY when
-- contract_data actually changes, and let the guard compare against it: a real
-- concurrent content edit still conflicts (lost-update protection intact), while
-- artifact/state timing can no longer false-trip it.

ALTER TABLE contracts
    ADD COLUMN IF NOT EXISTS content_updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

CREATE OR REPLACE FUNCTION contracts_content_updated_at_column()
    RETURNS TRIGGER AS $$
BEGIN
    IF NEW.contract_data IS DISTINCT FROM OLD.contract_data THEN
        NEW.content_updated_at = CURRENT_TIMESTAMP;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS contracts_content_updated_at ON contracts;
CREATE TRIGGER contracts_content_updated_at
    BEFORE UPDATE ON contracts
    FOR EACH ROW
EXECUTE FUNCTION contracts_content_updated_at_column();

-- Expose content_updated_at on the process-data view the optimistic-lock guard
-- reads (ReadProcessDataByDID). Definition mirrors
-- 20260706b_contracts_effective_views_new_terminal_states.sql; content_updated_at
-- is APPENDED last so CREATE OR REPLACE VIEW keeps the existing column order
-- (it only permits new trailing columns). Column order is irrelevant to the
-- name-based sqlx scan.
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
    contract_version,
    content_updated_at
FROM contracts;
