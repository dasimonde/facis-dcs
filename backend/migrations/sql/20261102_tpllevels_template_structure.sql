-- DCS-FR-TR-02, DCS-FR-TR-23: a template has a hierarchy level and may require
-- other templates (by DID). A contract from it needs a parent contract made
-- from one of the required templates.
ALTER TABLE contract_templates ADD COLUMN template_level TEXT NOT NULL DEFAULT 'AGREEMENT'
    CHECK (template_level IN ('FRAME_AGREEMENT', 'AGREEMENT', 'SUB_AGREEMENT', 'ADDENDUM'));
ALTER TABLE contract_templates ADD COLUMN required_templates TEXT[] NOT NULL DEFAULT '{}';
