-- Whether the stored contract PDF carries a PAdES signature. pdf_c2pa_state is
-- the lifecycle state the artifact asserts, which after signing can be
-- suspended, terminated or expired as well as active, so it does not say
-- whether the artifact is signed. Every later incremental update keeps the
-- signatures, so the flag only changes from false to true.
ALTER TABLE contracts ADD COLUMN IF NOT EXISTS pdf_signed BOOLEAN NOT NULL DEFAULT false;
