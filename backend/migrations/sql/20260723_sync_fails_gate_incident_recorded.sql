-- sync_fails.gate_incident_recorded distinguishes a retry entry created for a
-- federation trust-gate (ADR-19, agreement-credential) rejection from one
-- created for an unrelated reason (e.g. the PDF not being stored yet): the
-- same sync_fails row can be created by the latter and only later actually
-- become a trust-gate failure once the PDF is available, so "was this row
-- just INSERTed" alone cannot tell whether a trust-gate incident has already
-- been raised for it. Default FALSE: a freshly inserted row is only marked
-- TRUE when the insert itself is caused by a trust-gate failure.
ALTER TABLE sync_fails ADD COLUMN IF NOT EXISTS gate_incident_recorded BOOLEAN NOT NULL DEFAULT FALSE;
