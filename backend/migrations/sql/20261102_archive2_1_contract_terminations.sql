-- DCS-FR-CSA-16: the termination record of a terminated contract. The
-- archive shows the reason, the effective date and the role that initiated
-- the termination. A contract is terminated once, so one row per contract.
CREATE TABLE contract_terminations
(
    did             VARCHAR(255) PRIMARY KEY REFERENCES contracts (did) ON DELETE CASCADE,
    reason          TEXT         NOT NULL DEFAULT '',
    effective_at    TIMESTAMP    NOT NULL,
    terminated_by   VARCHAR(255) NOT NULL CHECK (terminated_by <> ''),
    initiating_role VARCHAR(64)  NOT NULL CHECK (initiating_role <> '')
);
