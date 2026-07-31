-- Drop c2pa_audit_log table and triggers (provenance is embedded in the PDF
-- chain via pdf-core's incremental updates; the table duplicated information
-- already available in the PDF and CWE event log).
DROP TRIGGER IF EXISTS trg_c2pa_audit_log_no_update ON c2pa_audit_log;
DROP TRIGGER IF EXISTS trg_c2pa_audit_log_no_delete ON c2pa_audit_log;
DROP FUNCTION IF EXISTS c2pa_audit_log_no_modify();
DROP TABLE IF EXISTS c2pa_audit_log;

-- Drop manifest-chain columns that were always written as NULL.
-- pdf_manifest_hash, pdf_manifest_ipfs_cid, prev_manifest_hash were part of
-- the old local JUMBF chain; pdf-core now owns all manifest construction.
ALTER TABLE contracts
    DROP COLUMN IF EXISTS pdf_manifest_hash,
    DROP COLUMN IF EXISTS pdf_manifest_ipfs_cid,
    DROP COLUMN IF EXISTS prev_manifest_hash;

ALTER TABLE contract_templates
    DROP COLUMN IF EXISTS pdf_manifest_hash,
    DROP COLUMN IF EXISTS pdf_manifest_ipfs_cid,
    DROP COLUMN IF EXISTS prev_manifest_hash;
