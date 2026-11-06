-- Counterparty Accept offer must tell the origin the round has opened
-- (OFFER_ACCEPTED on the negotiation outbound channel).
--
-- Only ADD VALUE here. Postgres forbids referencing a newly added enum value
-- in the same transaction (55P04); constraints/indexes that use OFFER_ACCEPTED
-- live in 20260743b_negotiation_offer_accepted_constraints.sql.

ALTER TYPE contract_negotiation_message_kind ADD VALUE IF NOT EXISTS 'OFFER_ACCEPTED';
