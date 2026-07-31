-- A completed signing ceremony now produces a JAdES (ETSI TS 119 182-1)
-- signature over the machine-readable JSON-LD contract representation
-- alongside the visible PAdES signature on the PDF, so each signature is
-- linked to BOTH representations (DCS-FR-SM-02, DCS-FR-SM-11). The JAdES is a
-- compact JWS (x5c + critical sigT) — stored as text next to the signature row.
ALTER TABLE contract_signatures
    ADD COLUMN IF NOT EXISTS jades_signature TEXT;
