-- DCS-FR-TR-05, DCS-IR-TR-08: the identity behind the last change of a
-- template. A history entry keeps the identity behind its revision.
ALTER TABLE contract_templates ADD COLUMN updated_by VARCHAR(255);
UPDATE contract_templates SET updated_by = created_by;
ALTER TABLE contract_templates ALTER COLUMN updated_by SET NOT NULL;

ALTER TABLE contract_templates_history ADD COLUMN updated_by VARCHAR(255);
UPDATE contract_templates_history SET updated_by = created_by;
ALTER TABLE contract_templates_history ALTER COLUMN updated_by SET NOT NULL;

-- A new template was last changed by its creator.
CREATE OR REPLACE FUNCTION contract_templates_default_updated_by()
    RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_by = COALESCE(NEW.updated_by, NEW.created_by);
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER contract_templates_default_updated_by
    BEFORE INSERT ON contract_templates
    FOR EACH ROW
EXECUTE FUNCTION contract_templates_default_updated_by();
