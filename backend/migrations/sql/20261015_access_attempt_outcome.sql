ALTER TABLE access_attempts ADD COLUMN outcome TEXT NOT NULL
    CHECK (outcome IN ('SUCCESS', 'DENIED', 'FAILED'));
CREATE INDEX access_attempts_page_idx ON access_attempts (attempted_at DESC, id DESC);
