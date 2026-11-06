-- Renewal-deadline ledger (DCS-FR-CWE-22, DCS-FR-CSA-20): one row per
-- (contract, expiry date) for which the expiry cron emitted
-- CONTRACT_RENEWAL_DUE. The primary key makes the notice fire once per
-- expiry date, also with several backend replicas. A changed exp_date has no
-- row yet, so it gets its own notice.
CREATE TABLE IF NOT EXISTS contract_renewal_notices
(
    did         VARCHAR(255) NOT NULL REFERENCES contracts (did) ON DELETE CASCADE,
    exp_date    TIMESTAMP    NOT NULL,
    notified_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    PRIMARY KEY (did, exp_date)
);
