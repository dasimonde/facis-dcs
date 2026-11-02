-- The optimistic lock of every template command compares content_updated_at.
-- It moves when a field that an editor changes moves, not only template_data.
CREATE OR REPLACE FUNCTION contract_templates_content_updated_at_column()
    RETURNS TRIGGER AS $$
BEGIN
    IF (NEW.template_data, NEW.name, NEW.description, NEW.template_type, NEW.party_type,
        NEW.jurisdiction, NEW.industry, NEW.template_level, NEW.required_templates)
       IS DISTINCT FROM
       (OLD.template_data, OLD.name, OLD.description, OLD.template_type, OLD.party_type,
        OLD.jurisdiction, OLD.industry, OLD.template_level, OLD.required_templates) THEN
        NEW.content_updated_at = CURRENT_TIMESTAMP;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
