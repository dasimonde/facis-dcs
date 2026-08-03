-- UC-14 / FR-SM-03 / ADR-31: the Power of Attorney presented at the ceremony is
-- retained alongside the PID presentation already stored in vp_token, so the
-- evidence behind an applied signature can be shipped to the counterparty and
-- verified there. Without it a peer receives only the dcs:hasPowerOfAttorney
-- claim on the party node, which it can read but cannot check.
ALTER TABLE signature_ceremonies ADD COLUMN poa_vp_token TEXT;
