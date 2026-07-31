-- The async PDF/C2PA pipeline caches its render results in pdf_* columns on
-- the same row (UpdatePDFState). The updated_at trigger treated those cache
-- writes as business updates, so a background render racing a user's edit
-- tripped optimistic concurrency ("contract was updated elsewhere") without
-- any business change having happened. updated_at now moves only when a
-- non-cache column actually changes.
CREATE OR REPLACE FUNCTION update_updated_at_column()
    RETURNS TRIGGER AS $$
BEGIN
    IF NEW.updated_at = OLD.updated_at
       AND to_jsonb(NEW) - 'updated_at' - 'pdf_ipfs_cid' - 'pdf_renderer_version' - 'pdf_c2pa_state' - 'pdf_payload_hash'
           IS DISTINCT FROM
           to_jsonb(OLD) - 'updated_at' - 'pdf_ipfs_cid' - 'pdf_renderer_version' - 'pdf_c2pa_state' - 'pdf_payload_hash'
    THEN
        NEW.updated_at = CURRENT_TIMESTAMP;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
