-- The archive evidence view previously surfaced the latest-requested
-- contract_deployments row; when the event-driven auto-dispatch fires after
-- an explicit deploy, that un-acknowledged row shadowed the acknowledged one
-- and evidence.deployment.receipt_hash/tsa_token vanished from the archive
-- entry. Prefer acknowledged deployments, then recency.
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
         INNER JOIN contract_archive_entries a
                    ON a.did = c.did
                        AND a.contract_version = c.contract_version
         LEFT JOIN LATERAL (
             SELECT cd.correlation_id, cd.content_hash, cd.status, cd.target_url, cd.requested_at,
                    cd.receipt_hash, cd.tsa_token, cd.acknowledged_at
             FROM contract_deployments cd
             WHERE cd.did = c.did AND cd.contract_version = c.contract_version
             ORDER BY (cd.acknowledged_at IS NOT NULL) DESC, cd.requested_at DESC
             LIMIT 1
         ) d ON true
WHERE a.archive_status <> 'DELETED';
