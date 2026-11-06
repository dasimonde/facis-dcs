-- DCS-FR-CWE-12: every termination records its reason.
ALTER TABLE contract_terminations ALTER COLUMN reason DROP DEFAULT;
ALTER TABLE contract_terminations
    ADD CONSTRAINT contract_terminations_reason_present CHECK (btrim(reason) <> '');
