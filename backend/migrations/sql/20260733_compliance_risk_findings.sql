-- One row per open compliance risk. The continuous-monitoring sweep re-derives
-- every risk from scratch on each run, so without this table it would re-report
-- the same finding on every run: an alert storm, and an audit trail in which a
-- single violation appears hundreds of times.
--
-- The key includes detail_hash, not just (contract_did, risk_type), because one
-- contract can carry several risks of the same type at once — one
-- MISSING_APPROVAL per outstanding approver, one UNAUTHORIZED_ACCESS per denied
-- actor, one CONTRACT_UNDERPERFORMANCE per breached metric. Keyed by type alone,
-- a second denied actor would be silently swallowed as "already reported". The
-- detail text is what distinguishes them and is derived only from persisted
-- facts (the approver, the actor, the metric, the dispatch), so it is stable
-- across sweeps; it is hashed because a btree key must stay bounded.
--
-- first_detected_at is deliberately reset when a resolved risk reoccurs: a
-- recurrence is a new incident, and the mean-time-to-detect measured against
-- this column (DCS-NFR-SEC-11) must describe that incident, not the previous
-- one. The superseded incidents are not lost — every detection is anchored in
-- the tamper-evident audit trail as a PAC_COMPLIANCE_RISK event.
CREATE TABLE IF NOT EXISTS compliance_risk_findings (

    contract_did      VARCHAR(255) NOT NULL,

    risk_type         VARCHAR(64)  NOT NULL,

    detail_hash       CHAR(64)     NOT NULL,

    -- Kept alongside the hash so an operator reading this table sees what was
    -- flagged without having to correlate it against the audit trail.
    detail            TEXT         NOT NULL,

    first_detected_at TIMESTAMP    NOT NULL,

    last_seen_at      TIMESTAMP    NOT NULL,

    resolved_at       TIMESTAMP,

    PRIMARY KEY (contract_did, risk_type, detail_hash)
);


-- Every sweep reads back the currently open findings to decide which ones have
-- disappeared and must be closed.
CREATE INDEX IF NOT EXISTS idx_compliance_risk_findings_open
    ON compliance_risk_findings (contract_did, risk_type)
    WHERE resolved_at IS NULL;
