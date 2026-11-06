-- A peer that refuses a termination notice or an erase request for good (it
-- holds no copy of the contract shared with this instance) ends the delivery:
-- refused_at is set and the row is not sent again. last_error keeps the
-- peer's answer.
ALTER TABLE contract_peer_terminations ADD COLUMN refused_at TIMESTAMPTZ;
ALTER TABLE contract_erasures ADD COLUMN refused_at TIMESTAMPTZ;
ALTER TABLE contract_erasures ADD COLUMN last_error TEXT;
