-- The internal compliance status is the result of the template policy audit at approval.
ALTER TABLE contract_templates ADD COLUMN compliance_status TEXT NOT NULL DEFAULT 'UNASSESSED'
    CHECK (compliance_status IN ('UNASSESSED', 'COMPLIANT', 'NON_COMPLIANT'));
