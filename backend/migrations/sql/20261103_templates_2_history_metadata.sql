-- DCS-FR-TR-05, DCS-FR-TR-21: a history entry keeps the classification and
-- the structure of the replaced revision.
ALTER TABLE contract_templates_history ADD COLUMN jurisdiction TEXT;
ALTER TABLE contract_templates_history ADD COLUMN industry TEXT;
ALTER TABLE contract_templates_history ADD COLUMN compliance_status TEXT NOT NULL DEFAULT 'UNASSESSED';
ALTER TABLE contract_templates_history ADD COLUMN template_level TEXT NOT NULL DEFAULT 'AGREEMENT';
ALTER TABLE contract_templates_history ADD COLUMN required_templates TEXT[] NOT NULL DEFAULT '{}';
