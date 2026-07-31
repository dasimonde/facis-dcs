-- ADR-33: the target system classifies, the DCS records.
--
-- contract_kpis.violation was the DCS's own derivation over the contract's
-- ODRL, and a false in it meant "not violated OR not evaluated" — the two were
-- indistinguishable in the column. The verdict now arrives with the report:
-- satisfied, violated, or not_evaluated, together with the @id of the ODRL rule
-- the target system concluded about, which travels to it verbatim in the
-- deployment envelope's odrl:policy.
--
-- Rows written before this are not reclassified. They carry a derived flag, not
-- a reported verdict, so they become not_evaluated — never satisfied, which is
-- what a green row would claim on a check that never ran.
ALTER TABLE contract_kpis DROP COLUMN violation;

ALTER TABLE contract_kpis
    ADD COLUMN verdict TEXT NOT NULL DEFAULT 'not_evaluated',
    ADD COLUMN rule_id TEXT;

ALTER TABLE contract_kpis
    ADD CONSTRAINT chk_contract_kpis_verdict
        CHECK (verdict IN ('satisfied', 'violated', 'not_evaluated'));

-- The default existed only to classify the pre-ADR-33 rows above. Every insert
-- from here on states the verdict it recorded.
ALTER TABLE contract_kpis ALTER COLUMN verdict DROP DEFAULT;

-- The compliance monitor (ADR-32) sweeps for reported breaches on every run.
CREATE INDEX idx_contract_kpis_verdict ON contract_kpis (verdict);
