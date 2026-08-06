-- ADR-34: this deployment serves the status list for the credentials it issues,
-- so the revocation bit has to live here.
--
-- Until now the bit lived in the XFSC statuslist-service: an allocation was
-- recorded locally (20260734) and the revocation was POSTed to that service,
-- which then served the bitstring unsigned. The list a verifier consults is now
-- built and signed from this table instead, which means the bit and the
-- allocation that names it are the same row and can no longer disagree — a
-- revocation that reached one and not the other used to leave a terminated
-- contract reading active with nothing to notice it.
--
-- NULL is "not revoked"; the timestamp is kept rather than a boolean because
-- the question asked of a status list after the fact is when a credential
-- stopped being valid, and a flag cannot answer it.
ALTER TABLE status_list_entries
    ADD COLUMN IF NOT EXISTS revoked_at TIMESTAMP NULL;

-- The list build reads every revoked entry of one list; nothing else selects on
-- this column.
CREATE INDEX IF NOT EXISTS idx_status_list_entries_revoked
    ON status_list_entries (list_id, entry_index)
 WHERE revoked_at IS NOT NULL;
