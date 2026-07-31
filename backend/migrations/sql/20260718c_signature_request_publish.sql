-- OID4VP Document-Retrieval signing request (ADR-12): the prepared to-be-signed
-- PDF and its digest must survive between publish and the wallet callback, so the
-- wallet fetches and signs exactly the bytes the DCS committed to. The publishing
-- signer's participant context is captured too, because the callback (the wallet's
-- direct_post) carries no JWT and finalize must attribute the signature to the
-- signer who published the request.
ALTER TABLE signature_ceremonies ADD COLUMN prepared_pdf         BYTEA;
ALTER TABLE signature_ceremonies ADD COLUMN prepared_pdf_sha256  VARCHAR(64);
ALTER TABLE signature_ceremonies ADD COLUMN request_nonce        VARCHAR(255);
ALTER TABLE signature_ceremonies ADD COLUMN request_expires_at   TIMESTAMP;
ALTER TABLE signature_ceremonies ADD COLUMN credential_type      VARCHAR(32);
ALTER TABLE signature_ceremonies ADD COLUMN published_by         VARCHAR(255);
ALTER TABLE signature_ceremonies ADD COLUMN published_holder_did VARCHAR(255);
ALTER TABLE signature_ceremonies ADD COLUMN published_roles      JSONB;
ALTER TABLE signature_ceremonies ADD COLUMN consumed_at          TIMESTAMP;
