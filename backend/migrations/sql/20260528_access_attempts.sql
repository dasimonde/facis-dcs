CREATE TABLE access_attempts (
    id          BIGSERIAL PRIMARY KEY,
    attempt_by     VARCHAR(255),
    ip_address  VARCHAR(45),
    attempted_at TIMESTAMP,
    success     BOOLEAN,
    service VARCHAR(64),
    method VARCHAR(64),
    roles TEXT,
    scope TEXT,
    did TEXT,
    justification TEXT
);

CREATE TABLE ip_lockouts (
    id           BIGSERIAL PRIMARY KEY,
    ip_address   TEXT        NOT NULL UNIQUE,
    locked_until TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
