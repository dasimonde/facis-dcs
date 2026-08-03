-- DCS-FR-SM-07/-17: "a field can never be signed twice" is what 20260714c
-- claimed and only indexed. The application check is a read the writers race
-- for; this is the constraint that holds whatever order they interleave in, so
-- a second SIGNED row for a field cannot exist even if a future change moves
-- the check back out from under the per-contract regeneration lock.
--
-- Partial, because the whole-column pair is legitimately repeated: a revoked
-- signature keeps its row (status REVOKED) and the field may be signed again,
-- and signatures predating 20260714c carry no field_name at all. Only the rows
-- the check itself looks at — SIGNED, with a field — are constrained, so no
-- legitimate existing row can violate it.
CREATE UNIQUE INDEX idx_contract_signatures_one_signed_per_field
    ON contract_signatures (contract_did, field_name)
 WHERE status = 'SIGNED' AND field_name IS NOT NULL;
