CREATE TYPE contract_template_state AS ENUM ('DRAFT', 'SUBMITTED', 'REJECTED', 'REVIEWED', 'APPROVED', 'REGISTERED', 'DELETED', 'DEPRECATED');


CREATE TYPE contract_template_type AS ENUM ('FRAME_CONTRACT', 'SUB_CONTRACT');


CREATE TABLE IF NOT EXISTS contract_templates
(
    did             VARCHAR(255),

    created_by      VARCHAR(255)   NOT NULL,
    created_at      TIMESTAMP      NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP      NOT NULL DEFAULT CURRENT_TIMESTAMP,

    state           contract_template_state NOT NULL,
    template_type   contract_template_type  NOT NULL,

    document_number VARCHAR(255),
    version         INT,
    name            VARCHAR(255),
    description     TEXT,
    template_data   JSONB DEFAULT '{}'::jsonb,
    search_vector   tsvector GENERATED ALWAYS AS (
        to_tsvector('english', template_data::text)
        ) STORED,

    CONSTRAINT pk_contract_templates PRIMARY KEY (did),
    CONSTRAINT chk_did_not_empty CHECK (did <> '')
);


CREATE INDEX idx_contract_templates_search ON contract_templates
    USING GIN (search_vector);


CREATE OR REPLACE FUNCTION update_updated_at_column()
    RETURNS TRIGGER AS
$$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;


CREATE TRIGGER contract_templates_update_updated_at
    BEFORE UPDATE
    ON contract_templates
    FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

------------------------------------------------------------------------------------------------------------------------

CREATE TYPE contract_template_review_task_state AS ENUM ('OPEN', 'APPROVED', 'REJECTED', 'VERIFIED');

CREATE TABLE IF NOT EXISTS contract_templates_review_task
(
    id              BIGSERIAL PRIMARY KEY,

    did             VARCHAR(255)      NOT NULL CHECK (did <> ''),

    state    contract_template_review_task_state NOT NULL,
    reviewer VARCHAR(255)      NOT NULL CHECK (reviewer <> ''),

    created_by      VARCHAR(255)      NOT NULL,
    created_at      TIMESTAMP         NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT fk_review_task_contract_template
        FOREIGN KEY (did)
            REFERENCES contract_templates (did)
);

------------------------------------------------------------------------------------------------------------------------

CREATE TYPE contract_template_approval_task_state AS ENUM ('OPEN', 'APPROVED', 'REJECTED', 'RESUBMITTED');


CREATE TABLE IF NOT EXISTS contract_templates_approval_task
(
    id              BIGSERIAL PRIMARY KEY,

    did             VARCHAR(255)        NOT NULL CHECK (did <> ''),

    state    contract_template_approval_task_state NOT NULL,
    approver VARCHAR(255)        NOT NULL CHECK (approver <> ''),

    created_by      VARCHAR(255)        NOT NULL,
    created_at      TIMESTAMP           NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT fk_approval_task_contract_template
        FOREIGN KEY (did)
            REFERENCES contract_templates (did)
);