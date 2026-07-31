CREATE TYPE contract_state AS ENUM ('DRAFT', 'NEGOTIATION', 'SUBMITTED', 'REJECTED', 'REVIEWED', 'APPROVED', 'SIGNED', 'TERMINATED', 'EXPIRED');
CREATE TYPE contract_expiration_policy AS ENUM ('RENEWAL', 'TERMINATION', 'ARCHIVING');


CREATE TABLE IF NOT EXISTS contracts
(
    did               VARCHAR(255),
    origin            VARCHAR(255),

    created_by        VARCHAR(255)   NOT NULL,
    created_at        TIMESTAMP      NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        TIMESTAMP      NOT NULL DEFAULT CURRENT_TIMESTAMP,

    start_date          TIMESTAMP,
    exp_date          TIMESTAMP,
    exp_policy        contract_expiration_policy,
    exp_notice_period INT,

    responsible     JSONB DEFAULT '{}'::jsonb,

    state             contract_state NOT NULL,

    contract_version  INT NOT NULL DEFAULT 1,

    template_did VARCHAR(255) NOT NULL,
    template_version INT NOT NULL,

    name              VARCHAR(255),
    description       TEXT,
    contract_data     JSONB DEFAULT '{}'::jsonb,
    search_vector     tsvector GENERATED ALWAYS AS (
        to_tsvector('english', contract_data::text)
        ) STORED,

    CONSTRAINT pk_contracts PRIMARY KEY (did),
    CONSTRAINT chk_did_not_empty CHECK (did <> '')
);


CREATE INDEX idx_contract_contracts_search ON contracts
    USING GIN (search_vector);


CREATE OR REPLACE FUNCTION update_updated_at_column()
    RETURNS TRIGGER AS $$
BEGIN
    IF NEW.updated_at = OLD.updated_at THEN
        NEW.updated_at = CURRENT_TIMESTAMP;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER contract_contracts_update_updated_at
    BEFORE UPDATE ON contracts
    FOR EACH ROW
EXECUTE FUNCTION update_updated_at_column();

------------------------------------------------------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS contract_history
(
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    did               VARCHAR(255),
    origin            VARCHAR(255),

    created_by        VARCHAR(255)   NOT NULL,
    created_at        TIMESTAMP      NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        TIMESTAMP      NOT NULL DEFAULT CURRENT_TIMESTAMP,

    start_date          TIMESTAMP,
    exp_date          TIMESTAMP,
    exp_policy        contract_expiration_policy,
    exp_notice_period INT,

    responsible     JSONB DEFAULT '{}'::jsonb,

    state             contract_state NOT NULL,

    contract_version  INT NOT NULL DEFAULT 1,

    template_did VARCHAR(255) NOT NULL,
    template_version INT NOT NULL,

    name              VARCHAR(255),
    description       TEXT,
    contract_data     JSONB DEFAULT '{}'::jsonb,
    search_vector     tsvector GENERATED ALWAYS AS (
        to_tsvector('english', contract_data::text)
        ) STORED,

    CONSTRAINT chk_did_not_empty CHECK (did <> '')
);

------------------------------------------------------------------------------------------------------------------------

CREATE TYPE contract_archive_status AS ENUM ('STORED', 'RETAINED', 'DELETION_REQUESTED', 'DELETED');

CREATE TABLE IF NOT EXISTS contract_archive_entries
(
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    did                VARCHAR(255) NOT NULL CHECK (did <> ''),
    contract_version   INT          NOT NULL,

    archive_status     contract_archive_status NOT NULL DEFAULT 'STORED',

    stored_by          VARCHAR(255) NOT NULL,
    stored_at          TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,

    contract_snapshot  JSONB NOT NULL,
    content_hash       TEXT NOT NULL CHECK (content_hash ~ '^sha256:[a-f0-9]{64}$'),
    snapshot_cid       TEXT NOT NULL CHECK (snapshot_cid <> ''),
    snapshot_cid_created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    signature_metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    credential_hashes  JSONB NOT NULL DEFAULT '{}'::jsonb,
    tsa_receipt        JSONB NOT NULL DEFAULT '{}'::jsonb,
    evidence           JSONB NOT NULL DEFAULT '{}'::jsonb,

    retention_until    TIMESTAMP,
    deleted_at         TIMESTAMP,
    deleted_by         VARCHAR(255),
    deletion_reason    TEXT,

    CONSTRAINT fk_contract_archive_entry_contract
        FOREIGN KEY (did)
            REFERENCES contracts (did),
    CONSTRAINT uq_contract_archive_entry_contract_version
        UNIQUE (did, contract_version),
    CONSTRAINT chk_contract_archive_entry_evidence_structure
        CHECK (evidence ? 'source' AND evidence ? 'snapshot_hash_algorithm'),
    CONSTRAINT chk_contract_archive_entry_deleted_state_metadata
        CHECK (
            archive_status <> 'DELETED'
                OR (
                deleted_at IS NOT NULL
                    AND deleted_by IS NOT NULL
                    AND deleted_by <> ''
                    AND deletion_reason IS NOT NULL
                    AND deletion_reason <> ''
                )
            ),
    CONSTRAINT chk_contract_archive_entry_active_state_metadata
        CHECK (
            archive_status IN ('DELETION_REQUESTED', 'DELETED')
                OR (
                deleted_at IS NULL
                    AND deleted_by IS NULL
                    AND deletion_reason IS NULL
                )
            )
);

CREATE INDEX idx_contract_archive_entries_status ON contract_archive_entries (archive_status);
CREATE INDEX idx_contract_archive_entries_did_version ON contract_archive_entries (did, contract_version);

CREATE OR REPLACE FUNCTION reject_contract_archive_entry_delete()
    RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'contract archive entries are append-only and cannot be deleted';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER contract_archive_entries_reject_delete
    BEFORE DELETE ON contract_archive_entries
    FOR EACH ROW
EXECUTE FUNCTION reject_contract_archive_entry_delete();

CREATE OR REPLACE FUNCTION protect_contract_archive_entry_immutable_fields()
    RETURNS TRIGGER AS $$
BEGIN
    IF NEW.did IS DISTINCT FROM OLD.did
        OR NEW.contract_version IS DISTINCT FROM OLD.contract_version
        OR NEW.stored_by IS DISTINCT FROM OLD.stored_by
        OR NEW.stored_at IS DISTINCT FROM OLD.stored_at
        OR NEW.contract_snapshot IS DISTINCT FROM OLD.contract_snapshot
        OR NEW.content_hash IS DISTINCT FROM OLD.content_hash
        OR NEW.snapshot_cid IS DISTINCT FROM OLD.snapshot_cid
        OR NEW.snapshot_cid_created_at IS DISTINCT FROM OLD.snapshot_cid_created_at
        OR NEW.signature_metadata IS DISTINCT FROM OLD.signature_metadata
        OR NEW.credential_hashes IS DISTINCT FROM OLD.credential_hashes
        OR NEW.tsa_receipt IS DISTINCT FROM OLD.tsa_receipt
        OR NEW.evidence IS DISTINCT FROM OLD.evidence THEN
        RAISE EXCEPTION 'immutable contract archive entry fields cannot be updated';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER contract_archive_entries_protect_immutable_fields
    BEFORE UPDATE ON contract_archive_entries
    FOR EACH ROW
EXECUTE FUNCTION protect_contract_archive_entry_immutable_fields();

CREATE OR REPLACE FUNCTION validate_contract_archive_entry_status_transition()
    RETURNS TRIGGER AS $$
BEGIN
    IF OLD.archive_status = 'DELETED'
        AND (
            NEW.archive_status IS DISTINCT FROM OLD.archive_status
                OR NEW.retention_until IS DISTINCT FROM OLD.retention_until
                OR NEW.deleted_at IS DISTINCT FROM OLD.deleted_at
                OR NEW.deleted_by IS DISTINCT FROM OLD.deleted_by
                OR NEW.deletion_reason IS DISTINCT FROM OLD.deletion_reason
            ) THEN
        RAISE EXCEPTION 'deleted contract archive entries cannot be changed';
    END IF;

    IF OLD.archive_status IS DISTINCT FROM NEW.archive_status THEN
        IF OLD.archive_status = 'STORED'
            AND NEW.archive_status NOT IN ('RETAINED', 'DELETION_REQUESTED', 'DELETED') THEN
            RAISE EXCEPTION 'invalid contract archive status transition from % to %', OLD.archive_status, NEW.archive_status;
        END IF;

        IF OLD.archive_status = 'RETAINED'
            AND NEW.archive_status NOT IN ('DELETION_REQUESTED', 'DELETED') THEN
            RAISE EXCEPTION 'invalid contract archive status transition from % to %', OLD.archive_status, NEW.archive_status;
        END IF;

        IF OLD.archive_status = 'DELETION_REQUESTED'
            AND NEW.archive_status <> 'DELETED' THEN
            RAISE EXCEPTION 'invalid contract archive status transition from % to %', OLD.archive_status, NEW.archive_status;
        END IF;
    END IF;

    IF NEW.archive_status IN ('STORED', 'RETAINED') THEN
        IF NEW.deleted_at IS NOT NULL
            OR NEW.deleted_by IS NOT NULL
            OR NEW.deletion_reason IS NOT NULL THEN
            RAISE EXCEPTION 'active contract archive entries cannot contain deletion metadata';
        END IF;
    END IF;

    IF NEW.archive_status = 'DELETION_REQUESTED' THEN
        IF NEW.deleted_at IS NOT NULL THEN
            RAISE EXCEPTION 'deletion-requested contract archive entries cannot contain deleted_at';
        END IF;
        IF NEW.deleted_by IS NULL
            OR NEW.deleted_by = ''
            OR NEW.deletion_reason IS NULL
            OR NEW.deletion_reason = '' THEN
            RAISE EXCEPTION 'deletion-requested contract archive entries require deleted_by and deletion_reason';
        END IF;
    END IF;

    IF NEW.archive_status = 'DELETED' THEN
        IF NEW.deleted_at IS NULL
            OR NEW.deleted_by IS NULL
            OR NEW.deleted_by = ''
            OR NEW.deletion_reason IS NULL
            OR NEW.deletion_reason = '' THEN
            RAISE EXCEPTION 'deleted contract archive entries require deleted_at, deleted_by, and deletion_reason';
        END IF;

        IF NEW.retention_until IS NOT NULL AND NEW.deleted_at < NEW.retention_until THEN
            RAISE EXCEPTION 'contract archive entries cannot be deleted before retention_until';
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER contract_archive_entries_validate_status_transition
    BEFORE UPDATE ON contract_archive_entries
    FOR EACH ROW
EXECUTE FUNCTION validate_contract_archive_entry_status_transition();

CREATE TABLE IF NOT EXISTS contract_archive_entry_events
(
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    archive_entry_id    uuid         NOT NULL,
    event_type          VARCHAR(100) NOT NULL CHECK (event_type <> ''),
    actor               VARCHAR(255) NOT NULL CHECK (actor <> ''),
    occurred_at         TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    reason              TEXT,
    event_data          JSONB        NOT NULL DEFAULT '{}'::jsonb,
    previous_event_hash TEXT         CHECK (previous_event_hash IS NULL OR previous_event_hash ~ '^sha256:[a-f0-9]{64}$'),
    event_hash          TEXT         NOT NULL CHECK (event_hash ~ '^sha256:[a-f0-9]{64}$'),

    CONSTRAINT fk_contract_archive_entry_event_entry
        FOREIGN KEY (archive_entry_id)
            REFERENCES contract_archive_entries (id),
    CONSTRAINT uq_contract_archive_entry_event_hash
        UNIQUE (event_hash)
);

CREATE INDEX idx_contract_archive_entry_events_entry ON contract_archive_entry_events (archive_entry_id);
CREATE INDEX idx_contract_archive_entry_events_occurred_at ON contract_archive_entry_events (occurred_at);

CREATE OR REPLACE FUNCTION reject_contract_archive_entry_event_modification()
    RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'contract archive entry events are append-only and cannot be modified';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER contract_archive_entry_events_reject_update
    BEFORE UPDATE ON contract_archive_entry_events
    FOR EACH ROW
EXECUTE FUNCTION reject_contract_archive_entry_event_modification();

CREATE TRIGGER contract_archive_entry_events_reject_delete
    BEFORE DELETE ON contract_archive_entry_events
    FOR EACH ROW
EXECUTE FUNCTION reject_contract_archive_entry_event_modification();

------------------------------------------------------------------------------------------------------------------------

CREATE OR REPLACE VIEW contracts_effective AS
SELECT
    did,
    origin,
    created_by,
    created_at,
    updated_at,
    start_date,
    exp_date,
    exp_policy,
    exp_notice_period,
    CASE
        WHEN exp_date <= CURRENT_TIMESTAMP
            AND state NOT IN ('TERMINATED', 'REJECTED', 'EXPIRED')
            THEN 'EXPIRED'::contract_state
        ELSE state
        END AS state,
    contract_version,
    name,
    description,
    contract_data,
    search_vector,
    responsible,
    template_did,
    template_version
FROM contracts;

------------------------------------------------------------------------------------------------------------------------

CREATE OR REPLACE VIEW contracts_effective_metadata AS
SELECT
    did,
    origin,
    created_by,
    created_at,
    updated_at,
    start_date,
    exp_date,
    exp_policy,
    exp_notice_period,
    CASE
        WHEN exp_date <= CURRENT_TIMESTAMP
            AND state NOT IN ('TERMINATED', 'REJECTED', 'EXPIRED')
            THEN 'EXPIRED'::contract_state
        ELSE state
        END AS state,
    contract_version,
    name,
    description,
    responsible,
    template_did,
    template_version
FROM contracts;

------------------------------------------------------------------------------------------------------------------------

CREATE OR REPLACE VIEW contracts_archive_metadata AS
SELECT
    c.did,
    c.created_by,
    c.created_at,
    c.updated_at,
    c.start_date,
    c.exp_date,
    c.exp_policy,
    c.exp_notice_period,
    c.state,
    c.contract_version,
    c.name,
    c.description,
    c.search_vector,
    c.responsible
FROM contracts_effective c
         INNER JOIN contract_archive_entries a
                    ON a.did = c.did
                        AND a.contract_version = c.contract_version
WHERE a.archive_status <> 'DELETED';

------------------------------------------------------------------------------------------------------------------------

CREATE OR REPLACE VIEW contracts_effective_process_data AS
SELECT
    did,
    origin,
    created_by,
    created_at,
    updated_at,
    start_date,
    exp_date,
    exp_policy,
    exp_notice_period,
    CASE
        WHEN exp_date <= CURRENT_TIMESTAMP
            AND state NOT IN ('TERMINATED', 'REJECTED', 'EXPIRED')
            THEN 'EXPIRED'::contract_state
        ELSE state
        END AS state,
    contract_version
FROM contracts;

------------------------------------------------------------------------------------------------------------------------

CREATE TYPE contract_review_task_state AS ENUM ('OPEN', 'APPROVED', 'REJECTED');

CREATE TABLE IF NOT EXISTS contract_review_task
(
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    did             VARCHAR(255) NOT NULL CHECK (did <> ''),

    state    contract_review_task_state NOT NULL,
    reviewer VARCHAR(255)      NOT NULL CHECK (reviewer <> ''),

    created_by VARCHAR(255) NOT NULL,
    created_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT fk_review_task_contract
        FOREIGN KEY (did)
            REFERENCES contracts (did)
);

------------------------------------------------------------------------------------------------------------------------

CREATE TYPE contract_approval_task_state AS ENUM ('OPEN', 'APPROVED', 'REJECTED');

CREATE TABLE IF NOT EXISTS contract_approval_task
(
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    did             VARCHAR(255) NOT NULL CHECK (did <> ''),

    state    contract_approval_task_state NOT NULL,
    approver VARCHAR(255)        NOT NULL CHECK (approver <> ''),

    created_by VARCHAR(255) NOT NULL,
    created_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT fk_approval_task_contract
        FOREIGN KEY (did)
            REFERENCES contracts (did)
);

------------------------------------------------------------------------------------------------------------------------

CREATE TYPE contract_negotiation_task_state AS ENUM ('OPEN', 'ACCEPTED');

CREATE TABLE IF NOT EXISTS contract_negotiation_task
(
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    did             VARCHAR(255) NOT NULL CHECK (did <> ''),

    state    contract_negotiation_task_state NOT NULL,
    negotiator VARCHAR(255)        NOT NULL CHECK (negotiator <> ''),

    created_by VARCHAR(255) NOT NULL,
    created_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT fk_contract_negotiation_task
        FOREIGN KEY (did)
            REFERENCES contracts (did)
);

------------------------------------------------------------------------------------------------------------------------

CREATE TYPE contract_negotiation_decision AS ENUM ('ACCEPTED', 'REJECTED', 'CLOSED');

CREATE TABLE IF NOT EXISTS contract_negotiations
(
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    did                 VARCHAR(255) NOT NULL CHECK (did <> ''),
    contract_version    INT NOT NULL,

    change_request      JSONB DEFAULT '{}'::jsonb,

    created_by VARCHAR(255) NOT NULL,
    created_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT fk_contracts
        FOREIGN KEY (did)
            REFERENCES contracts (did)
);

CREATE TABLE IF NOT EXISTS contract_negotiation_decisions
(
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    negotiation_id      uuid,

    negotiator         VARCHAR(255) NOT NULL,
    decision            contract_negotiation_decision,
    rejection_reason    TEXT,

    CONSTRAINT fk_contract_negotiations
        FOREIGN KEY (negotiation_id)
            REFERENCES contract_negotiations (id)
            ON DELETE CASCADE
);
