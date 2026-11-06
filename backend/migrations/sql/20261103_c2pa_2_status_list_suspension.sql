-- A suspension (a revoked signature, DCS-OR-C2PA-005) is a status of its own:
-- it ends when the contract is signed again, and a verifier must be able to
-- tell it from a termination. The served list carries two bits per entry: 1 for
-- an entry whose revoked_at is set, 2 for one whose suspended_at is set. A
-- revocation is final and wins over a suspension.
ALTER TABLE status_list_entries
    ADD COLUMN IF NOT EXISTS suspended_at TIMESTAMP NULL;

CREATE INDEX IF NOT EXISTS idx_status_list_entries_suspended
    ON status_list_entries (list_id, entry_index)
 WHERE suspended_at IS NOT NULL;
