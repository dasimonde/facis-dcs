-- DCS-FR-CSA-18: access attempts, refused ones included, are part of the
-- immutable audit log. Rows are append-only.
CREATE OR REPLACE FUNCTION reject_access_attempt_mutation()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'access_attempts is append-only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER access_attempts_append_only
BEFORE UPDATE OR DELETE ON access_attempts
FOR EACH ROW EXECUTE FUNCTION reject_access_attempt_mutation();
