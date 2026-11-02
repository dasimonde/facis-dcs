-- A counterparty signature can arrive after this instance archived the
-- contract (DCS-FR-CSA-08). The archive entry stays immutable, with two
-- permitted changes: the erasure of the snapshot, and the append of peer
-- signatures to signature_metadata.peer_signatures. An append keeps every
-- other key of signature_metadata and every earlier peer signature in its
-- position.
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
        OR NEW.credential_hashes IS DISTINCT FROM OLD.credential_hashes
        OR NEW.tsa_receipt IS DISTINCT FROM OLD.tsa_receipt
        OR NEW.evidence IS DISTINCT FROM OLD.evidence THEN
        RAISE EXCEPTION 'immutable contract archive entry fields cannot be updated';
    END IF;
    IF NEW.signature_metadata IS DISTINCT FROM OLD.signature_metadata
        AND NOT (
            (NEW.signature_metadata - 'peer_signatures') = (OLD.signature_metadata - 'peer_signatures')
            AND jsonb_typeof(NEW.signature_metadata -> 'peer_signatures') = 'array'
            AND jsonb_typeof(COALESCE(OLD.signature_metadata -> 'peer_signatures', '[]'::jsonb)) = 'array'
            AND jsonb_array_length(NEW.signature_metadata -> 'peer_signatures')
                > jsonb_array_length(COALESCE(OLD.signature_metadata -> 'peer_signatures', '[]'::jsonb))
            AND COALESCE(OLD.signature_metadata -> 'peer_signatures', '[]'::jsonb) = COALESCE((
                SELECT jsonb_agg(element ORDER BY position)
                FROM jsonb_array_elements(NEW.signature_metadata -> 'peer_signatures') WITH ORDINALITY AS appended(element, position)
                WHERE position <= jsonb_array_length(COALESCE(OLD.signature_metadata -> 'peer_signatures', '[]'::jsonb))
            ), '[]'::jsonb)
        ) THEN
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
