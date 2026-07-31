-- Deployment/execution-evidence/KPI tables (contract deployment,
-- UC-05-01). One row per POST /contract/deploy dispatch (manual or
-- event-driven), keyed by a correlation_id the target echoes back in its
-- ack/status/KPI callbacks.

CREATE TABLE contract_deployments (
    id               BIGSERIAL PRIMARY KEY,
    did              VARCHAR(255) NOT NULL REFERENCES contracts(did),
    contract_version INT NOT NULL,
    correlation_id   VARCHAR(255) NOT NULL UNIQUE,
    content_hash     VARCHAR(255) NOT NULL,
    target_url       TEXT,
    status           VARCHAR(32) NOT NULL DEFAULT 'DISPATCHED',
    requested_by     VARCHAR(255) NOT NULL,
    requested_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    acknowledged_at  TIMESTAMP,
    receipt_hash     VARCHAR(255),
    tsa_token        TEXT
);

CREATE INDEX idx_contract_deployments_did ON contract_deployments(did);
CREATE INDEX idx_contract_deployments_correlation_id ON contract_deployments(correlation_id);

-- One row per KPI value reported via POST /contract/deployment/callback for
-- an ACTIVE contract, with a violation flag set when the reported value
-- crosses the contract's own ODRL SLA constraint for that metric.
CREATE TABLE contract_kpis (
    id               BIGSERIAL PRIMARY KEY,
    did              VARCHAR(255) NOT NULL REFERENCES contracts(did),
    correlation_id   VARCHAR(255),
    metric           VARCHAR(255) NOT NULL,
    value            TEXT NOT NULL,
    observed_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    violation        BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX idx_contract_kpis_did ON contract_kpis(did);

------------------------------------------------------------------------------------------------------------------------

-- contracts_archive_metadata grows the archive entry's evidence blob so
-- archive search/retrieve responses can surface the deployment sub-object
-- (correlation_id, payload_hash, receipt_hash, tsa_token, activated_at).
CREATE OR REPLACE VIEW contracts_archive_metadata AS
SELECT
    c.did,
    c.created_by,
    c.created_at,
    c.updated_at,
    c.start_date,
    c.exp_date,
    c.exp_policy,
    c.exp_notice_period,
    c.state,
    c.contract_version,
    c.name,
    c.description,
    c.search_vector,
    c.responsible,
    a.evidence
FROM contracts_effective c
         INNER JOIN contract_archive_entries a
                    ON a.did = c.did
                        AND a.contract_version = c.contract_version
WHERE a.archive_status <> 'DELETED';
