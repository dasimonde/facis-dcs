-- DCS-FR-CSA-12: every live archive entry is listed with its own archived
-- version. A countersigned PDF from a peer increases the contract's current
-- version after this instance archived the version it signed; the entry stays
-- retrievable, searchable and annotatable. The definition is the one of
-- 20260714_archive_summary_tags.sql except for the join on the DID only and
-- the entry's own contract_version.
DROP VIEW IF EXISTS contracts_archive_metadata;
CREATE VIEW contracts_archive_metadata AS
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
    a.contract_version,
    c.name,
    c.description,
    c.search_vector,
    c.responsible,
    a.summary AS archive_summary,
    a.tags    AS archive_tags,
    CASE
        WHEN d.correlation_id IS NULL THEN a.evidence
        ELSE jsonb_set(
            COALESCE(a.evidence, '{}'::jsonb),
            '{deployment}',
            jsonb_strip_nulls(jsonb_build_object(
                'correlation_id', d.correlation_id,
                'payload_hash', d.content_hash,
                'status', d.status,
                'target_url', d.target_url,
                'dispatched_at', to_char(d.requested_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"'),
                'receipt_hash', d.receipt_hash,
                'tsa_token', d.tsa_token,
                'activated_at', to_char(d.acknowledged_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
            )),
            true
        )
    END AS evidence
FROM contracts_effective c
         INNER JOIN contract_archive_entries a ON a.did = c.did
         LEFT JOIN LATERAL (
             SELECT cd.correlation_id, cd.content_hash, cd.status, cd.target_url, cd.requested_at,
                    cd.receipt_hash, cd.tsa_token, cd.acknowledged_at
             FROM contract_deployments cd
             WHERE cd.did = c.did AND cd.contract_version = c.contract_version
             ORDER BY (cd.acknowledged_at IS NOT NULL) DESC, cd.requested_at DESC
             LIMIT 1
         ) d ON true
WHERE a.archive_status <> 'DELETED';
