CREATE TABLE access_attempts (
    id          BIGSERIAL PRIMARY KEY,
    attempt_by     VARCHAR(255),
    ip_address  VARCHAR(45),
    attempted_at TIMESTAMP,
    success     BOOLEAN,
    service VARCHAR(64),
    method VARCHAR(64)
);

CREATE TABLE ip_lockouts (
    id           BIGSERIAL PRIMARY KEY,
    ip_address   TEXT        NOT NULL UNIQUE,
    locked_until TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);