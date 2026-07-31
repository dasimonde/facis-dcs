-- Contract-state-machine refactor: add the first-class
-- offer/acceptance states around the existing contract_state enum. Per the
-- greenfield rule this is additive-only (ALTER TYPE ... ADD VALUE) rather
-- than a drop/recreate, since contract_state already backs live columns
-- (contracts.state, contract_history.state) and Postgres requires enum
-- values to be added outside of the transaction that first uses them —
-- this migration only adds values, it does not reference them.
--
-- New states (see docs/adr-2-contract-state-machine.md):
--   OFFERED   - contract has been transmitted to the counterparty (DRAFT -> OFFERED)
--   WITHDRAWN - initiator retracted the contract before approval (terminal)
--   ACTIVE    - post-signing execution/deployment state (not entered by any
--               command in this workstream's scope)
--   REVOKED   - signature/credential revocation state (not entered by any
--               command in this workstream's scope)
ALTER TYPE contract_state ADD VALUE IF NOT EXISTS 'OFFERED';
ALTER TYPE contract_state ADD VALUE IF NOT EXISTS 'WITHDRAWN';
ALTER TYPE contract_state ADD VALUE IF NOT EXISTS 'ACTIVE';
ALTER TYPE contract_state ADD VALUE IF NOT EXISTS 'REVOKED';
