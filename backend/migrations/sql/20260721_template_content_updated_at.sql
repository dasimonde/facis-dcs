-- Same defect as 20260720_contract_content_updated_at.sql, on the template side:
-- the optimistic-lock guard compared the caller's fetched timestamp against
-- contract_templates.updated_at, which the contract_templates_update_updated_at
-- trigger moves on ANY write to the row — including the background PDF write
-- (UpdatePDFState: pdf_ipfs_cid/pdf_renderer_version/pdf_c2pa_state/…). A caller
-- that read a fresh template and submitted it immediately therefore lost the
-- race against its own artifact generation and was refused with "contract
-- template was updated elsewhere, please reload", though nothing about the
-- template's content had changed. Track a content_updated_at that moves only on
-- a real template_data edit and let the guard compare against that: a genuine
-- concurrent content edit still conflicts, artifact timing no longer can.

ALTER TABLE contract_templates
    ADD COLUMN IF NOT EXISTS content_updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP;

CREATE OR REPLACE FUNCTION contract_templates_content_updated_at_column()
    RETURNS TRIGGER AS $$
BEGIN
    IF NEW.template_data IS DISTINCT FROM OLD.template_data THEN
        NEW.content_updated_at = CURRENT_TIMESTAMP;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS contract_templates_content_updated_at ON contract_templates;
CREATE TRIGGER contract_templates_content_updated_at
    BEFORE UPDATE ON contract_templates
    FOR EACH ROW
EXECUTE FUNCTION contract_templates_content_updated_at_column();
