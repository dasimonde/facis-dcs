-- DCS-FR-CSA-11: archived contracts carry a summary (manual or
-- system-generated) and user-assigned tags for thematic categorization and
-- discovery. The annotation lives on the archive entry itself — the entry's
-- snapshot/evidence columns stay immutable (see
-- protect_contract_archive_entry_immutable_fields, which deliberately does
-- NOT list summary/tags), so annotating never touches archived evidence.
ALTER TABLE contract_archive_entries
    ADD COLUMN summary TEXT,
    ADD COLUMN tags    JSONB NOT NULL DEFAULT '[]'::jsonb
        CONSTRAINT chk_contract_archive_entry_tags_array
            CHECK (jsonb_typeof(tags) = 'array');

CREATE INDEX idx_contract_archive_entries_tags
    ON contract_archive_entries USING GIN (tags);

-- Recreate the archive metadata view (definition carried over from
-- 20260713_archive_evidence_prefer_acknowledged.sql) with the annotation
-- columns exposed as archive_summary/archive_tags. DROP + CREATE because
-- CREATE OR REPLACE VIEW cannot insert columns before existing ones.
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
    c.contract_version,
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
