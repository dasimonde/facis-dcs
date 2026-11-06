-- search_vector is a stored generated column. In a BEFORE UPDATE trigger its NEW
-- value differs from OLD, so comparing to_jsonb(NEW) with to_jsonb(OLD) found a
-- change on every write. The background PDF cache write then moved updated_at,
-- and an edit based on a fresh read was refused as "updated elsewhere".
-- search_vector derives from template_data or contract_data, which stay in the
-- comparison.
CREATE OR REPLACE FUNCTION update_updated_at_column()
    RETURNS TRIGGER AS $$
BEGIN
    IF NEW.updated_at = OLD.updated_at
       AND to_jsonb(NEW) - 'updated_at' - 'search_vector' - 'pdf_ipfs_cid' - 'pdf_renderer_version' - 'pdf_c2pa_state' - 'pdf_payload_hash'
           IS DISTINCT FROM
           to_jsonb(OLD) - 'updated_at' - 'search_vector' - 'pdf_ipfs_cid' - 'pdf_renderer_version' - 'pdf_c2pa_state' - 'pdf_payload_hash'
    THEN
        NEW.updated_at = CURRENT_TIMESTAMP;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION contracts_update_updated_at_column()
    RETURNS TRIGGER AS $$
BEGIN
    IF NEW.updated_at IS DISTINCT FROM OLD.updated_at
       OR to_jsonb(NEW) - 'updated_at' - 'search_vector' - 'pdf_ipfs_cid' - 'pdf_renderer_version' - 'pdf_c2pa_state' - 'pdf_payload_hash'
           IS DISTINCT FROM
       to_jsonb(OLD) - 'updated_at' - 'search_vector' - 'pdf_ipfs_cid' - 'pdf_renderer_version' - 'pdf_c2pa_state' - 'pdf_payload_hash'
    THEN
        IF NEW.updated_at IS DISTINCT FROM OLD.updated_at THEN
            NEW.updated_at = GREATEST(
                NEW.updated_at,
                NEW.content_updated_at,
                OLD.updated_at + INTERVAL '1 microsecond'
            );
        ELSE
            NEW.updated_at = GREATEST(
                clock_timestamp(),
                NEW.content_updated_at,
                OLD.updated_at + INTERVAL '1 microsecond'
            );
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
