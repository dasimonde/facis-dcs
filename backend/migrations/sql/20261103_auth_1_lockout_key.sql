-- DCS-FR-UC-01-4: an attempt counts toward a lockout only when it carries the
-- key it locks (a client address or a credential holder key). Refusals of a
-- locked caller, expired tokens and missing tokens carry no key.
ALTER TABLE access_attempts ADD COLUMN lockout_key TEXT;
CREATE INDEX access_attempts_lockout_idx ON access_attempts (lockout_key, attempted_at)
    WHERE lockout_key IS NOT NULL;

-- A lock row stays after its end: locked_until is the point from which new
-- invalid attempts count again. An unlock sets it to the unlock time.
ALTER TABLE ip_lockouts RENAME TO lockouts;
ALTER TABLE lockouts RENAME COLUMN ip_address TO lockout_key;
