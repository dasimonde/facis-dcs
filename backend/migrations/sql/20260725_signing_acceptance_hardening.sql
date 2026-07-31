-- Signing acceptance hardening (ADR-20, supersedes ADR-12's acceptance-path
-- clauses): pin the exact to-be-signed bytes and the contract-side metadata
-- `finalize` needs at EVERY prepare (wallet ceremony and desktop
-- /signature/prepare alike), so submit validates against committed bytes
-- instead of re-deriving them, and can run with no side effects.
ALTER TABLE signature_ceremonies ADD COLUMN pinned_payload           BYTEA;
ALTER TABLE signature_ceremonies ADD COLUMN pinned_payload_sha256    VARCHAR(64);
ALTER TABLE signature_ceremonies ADD COLUMN pinned_content_hash      VARCHAR(64);
ALTER TABLE signature_ceremonies ADD COLUMN pinned_renderer_version  VARCHAR(64);
ALTER TABLE signature_ceremonies ADD COLUMN pinned_signed_count      INT;
ALTER TABLE signature_ceremonies ADD COLUMN pinned_contract_version  INT;

-- The contract's OWN declared signature-level requirement for this field
-- (dcs:requiredCredentialType on the dcs:SignatureField node, default AES),
-- pinned at prepare so submit gates on it instead of on the caller-supplied
-- credential_type (SM-01 per-contract level enforcement).
ALTER TABLE signature_ceremonies ADD COLUMN required_credential_type VARCHAR(32);

-- Sole control (eIDAS Art. 26c): the signing certificate's subject and serial,
-- recorded once the signature validates, so the Signature Compliance Viewer
-- (SM-26) can show the certificate that actually produced the signature and a
-- repeat signatory's certificate identity can be checked for consistency.
ALTER TABLE signature_ceremonies ADD COLUMN signer_cert_subject VARCHAR(1024);
ALTER TABLE signature_ceremonies ADD COLUMN signer_cert_serial  VARCHAR(255);
