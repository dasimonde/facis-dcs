-- A negotiation task is a fact about one ROUND, not about a contract. Each
-- accepted redline bumps contract_version and starts a new round, so the task
-- carries the version it was minted for; the round predicates (is this instance
-- a negotiator, are any tasks still open, accept my task) all scope to it.
--
-- The unique index is the idempotency mechanism: accepting the same offer twice
-- inserts once (the repository's Create uses ON CONFLICT DO NOTHING).
--
-- Existing rows are stamped with the round their contract actually stands on.
-- Landing them all on 1 strands every in-flight contract past version 1: the
-- originator's only mint is create.go at version 1, acceptoffer refuses the
-- origin with ErrNotAParty and negotiate gates it on IsValidNegotiator, so a
-- task that falls behind on a contract this instance authored can never be
-- re-minted and the party is locked out of its own negotiation.
ALTER TABLE contract_negotiation_task
    ADD COLUMN contract_version INT NOT NULL DEFAULT 1;

UPDATE contract_negotiation_task t
    SET contract_version = c.contract_version
    FROM contracts c
    WHERE c.did = t.did;

ALTER TABLE contract_negotiation_task
    ALTER COLUMN contract_version DROP DEFAULT;

CREATE UNIQUE INDEX uq_contract_negotiation_task_round
    ON contract_negotiation_task (did, negotiator, contract_version);
