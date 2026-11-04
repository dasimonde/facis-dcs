CREATE OR REPLACE FUNCTION contract_templates_content_updated_at_column()
    RETURNS TRIGGER AS $$
BEGIN
    IF (NEW.template_data, NEW.name, NEW.description, NEW.template_type, NEW.party_type,
        NEW.template_level, NEW.required_templates)
       IS DISTINCT FROM
       (OLD.template_data, OLD.name, OLD.description, OLD.template_type, OLD.party_type,
        OLD.template_level, OLD.required_templates) THEN
        NEW.content_updated_at = CURRENT_TIMESTAMP;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

ALTER TABLE contract_templates DROP COLUMN jurisdiction, DROP COLUMN industry;
ALTER TABLE contract_templates_history DROP COLUMN jurisdiction, DROP COLUMN industry;
