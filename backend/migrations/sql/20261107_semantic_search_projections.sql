ALTER TABLE contract_templates ADD COLUMN search_projection JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE contracts ADD COLUMN search_projection JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE contract_archive_entries ADD COLUMN search_projection JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE INDEX contract_templates_search_projection_idx ON contract_templates USING GIN (search_projection);
CREATE INDEX contracts_search_projection_idx ON contracts USING GIN (search_projection);
CREATE INDEX contract_archive_search_projection_idx ON contract_archive_entries USING GIN (search_projection);

CREATE FUNCTION protect_archive_search_projection() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.contract_snapshot IS DISTINCT FROM OLD.contract_snapshot
       AND NEW.contract_snapshot = '{}'::jsonb AND NEW.search_projection <> '{}'::jsonb THEN
        RAISE EXCEPTION 'archive erasure must clear its search projection';
    END IF;
    IF NEW.search_projection IS DISTINCT FROM OLD.search_projection AND NOT (
        NEW.search_projection = '{}'::jsonb AND NEW.contract_snapshot = '{}'::jsonb
        AND EXISTS (SELECT 1 FROM content_encryption_keys k
                    WHERE k.scope_kind = 'contract' AND k.scope_id = NEW.did AND k.shredded_at IS NOT NULL)
    ) THEN
        RAISE EXCEPTION 'immutable contract archive search projection cannot be updated';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER contract_archive_entries_protect_search_projection
    BEFORE UPDATE ON contract_archive_entries
    FOR EACH ROW EXECUTE FUNCTION protect_archive_search_projection();
