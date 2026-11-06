-- Erasure (DCS-NFR-SEC-13, DCS-NFR-COMP-03, ADR-28). A shred destroys the
-- wrapped CEK itself. The shredded row keeps only the destruction record.
ALTER TABLE content_encryption_keys ALTER COLUMN wrapped_cek DROP NOT NULL;
-- A row shredded before this migration still holds its wrapped key.
UPDATE content_encryption_keys SET wrapped_cek = NULL WHERE shredded_at IS NOT NULL;
ALTER TABLE content_encryption_keys
    ADD CONSTRAINT chk_cek_shredded_has_no_key
        CHECK ((shredded_at IS NULL) = (wrapped_cek IS NOT NULL));

-- The archive entry's identifiers (DCS-FR-CSA-09) and evidence stay
-- immutable. The one permitted change to the snapshot is the erasure: it
-- becomes an empty object after the contract's keys are shredded.
-- content_hash stays as the evidence of what was archived.
CREATE OR REPLACE FUNCTION protect_contract_archive_entry_immutable_fields()
    RETURNS TRIGGER AS $$
BEGIN
    IF NEW.id IS DISTINCT FROM OLD.id
        OR NEW.did IS DISTINCT FROM OLD.did
        OR NEW.contract_version IS DISTINCT FROM OLD.contract_version
        OR NEW.stored_by IS DISTINCT FROM OLD.stored_by
        OR NEW.stored_at IS DISTINCT FROM OLD.stored_at
        OR NEW.content_hash IS DISTINCT FROM OLD.content_hash
        OR NEW.snapshot_cid IS DISTINCT FROM OLD.snapshot_cid
        OR NEW.snapshot_cid_created_at IS DISTINCT FROM OLD.snapshot_cid_created_at
        OR NEW.signature_metadata IS DISTINCT FROM OLD.signature_metadata
        OR NEW.credential_hashes IS DISTINCT FROM OLD.credential_hashes
        OR NEW.tsa_receipt IS DISTINCT FROM OLD.tsa_receipt
        OR NEW.evidence IS DISTINCT FROM OLD.evidence THEN
        RAISE EXCEPTION 'immutable contract archive entry fields cannot be updated';
    END IF;
    IF NEW.contract_snapshot IS DISTINCT FROM OLD.contract_snapshot
        AND NOT (
            NEW.contract_snapshot = '{}'::jsonb
            AND EXISTS (
                SELECT 1 FROM content_encryption_keys k
                WHERE k.scope_kind = 'contract' AND k.scope_id = NEW.did AND k.shredded_at IS NOT NULL)
        ) THEN
        RAISE EXCEPTION 'immutable contract archive entry fields cannot be updated';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
