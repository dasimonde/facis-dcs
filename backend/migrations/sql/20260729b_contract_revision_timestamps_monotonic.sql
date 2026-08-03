-- CURRENT_TIMESTAMP is fixed at transaction start. A long-running transaction
-- could therefore commit after a newer contract update and move updated_at
-- backwards, making a freshly returned RFC3339 concurrency token appear to be
-- from the future. Contract revision clocks must be commit-order monotonic.
CREATE OR REPLACE FUNCTION contracts_update_updated_at_column()
    RETURNS TRIGGER AS $$
BEGIN
    IF NEW.updated_at IS DISTINCT FROM OLD.updated_at
       OR to_jsonb(NEW) - 'updated_at' - 'pdf_ipfs_cid' - 'pdf_renderer_version' - 'pdf_c2pa_state' - 'pdf_payload_hash'
           IS DISTINCT FROM
       to_jsonb(OLD) - 'updated_at' - 'pdf_ipfs_cid' - 'pdf_renderer_version' - 'pdf_c2pa_state' - 'pdf_payload_hash'
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

DROP TRIGGER IF EXISTS contract_contracts_update_updated_at ON contracts;

-- The content revision participates in workflow-gate stale checks and must obey
-- the same monotonic guarantee when contract_data changes.
CREATE OR REPLACE FUNCTION contracts_content_updated_at_column()
    RETURNS TRIGGER AS $$
BEGIN
    IF NEW.contract_data IS DISTINCT FROM OLD.contract_data THEN
        NEW.content_updated_at =
            GREATEST(clock_timestamp(), OLD.content_updated_at + INTERVAL '1 microsecond');
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- PostgreSQL fires same-event triggers alphabetically. The existing
-- contracts_content_updated_at trigger therefore runs before this revision
-- trigger, allowing updated_at to include the final content_updated_at value.
DROP TRIGGER IF EXISTS contracts_revision_updated_at ON contracts;
CREATE TRIGGER contracts_revision_updated_at
    BEFORE UPDATE ON contracts
    FOR EACH ROW
EXECUTE FUNCTION contracts_update_updated_at_column();
