-- ADR-31 / DCS-FR-SM-08: the signing summary credential this instance issued for
-- a signature, retained so it can travel to the counterparty on the wire.
--
-- It is embedded in the PDF at apply time, but only the FIRST signer's bundle
-- survives there: pdf-core's extractor returns the last attachment, and adding a
-- second one would mutate a document that already carries a PAdES signature.
-- Shipping it beside the Power of Attorney leaves the signed artefact untouched.
ALTER TABLE signature_ceremonies ADD COLUMN summary_vc TEXT;
