-- Folding a negotiation round is last-accepted-wins, so an accepted change
-- request can contribute nothing to the merged version while its decision row
-- still reads ACCEPTED. superseded_by carries the annotation that says so:
-- a JSON array of {"superseded_by": <negotiation id>, "fields": [...]}, one
-- entry per later request that overwrote part of this one. NULL means the
-- round has not folded yet, or that nothing of this request was discarded.
--
-- Held as JSONB rather than a single foreign key because different fields of
-- one request can be beaten by different later requests.
ALTER TABLE contract_negotiations
    ADD COLUMN IF NOT EXISTS superseded_by JSONB;
