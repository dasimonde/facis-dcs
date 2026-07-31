-- Records the moment a signature's signing certificate was found revoked in
-- the CRL, so /signature/validate can report a certificate-revocation finding
-- distinct from the business-level REVOKED signature status (DCS-OR-C2PA-007).
ALTER TABLE contract_signatures ADD COLUMN cert_revoked_at TIMESTAMPTZ;
