ALTER TABLE signature_ceremonies
    ADD COLUMN IF NOT EXISTS poa_subject_did VARCHAR(255);
