-- DCS-FR-TR-09 Template Provenance and Versioning: one signed W3C VC per
-- registered template version, carrying the creator/reviewer/approver/
-- registrar claims and linked to the previous version's credential.
CREATE TABLE IF NOT EXISTS template_provenance_credentials
(
    id             BIGSERIAL PRIMARY KEY,

    did            VARCHAR(255) NOT NULL CHECK (did <> ''),
    version        INT          NOT NULL,

    vc_id          VARCHAR(255) NOT NULL,
    previous_vc_id VARCHAR(255),

    credential     JSONB        NOT NULL,

    created_at     TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT uq_template_provenance_version UNIQUE (did, version),

    CONSTRAINT fk_template_provenance_template
        FOREIGN KEY (did)
            REFERENCES contract_templates (did)
            ON DELETE CASCADE
);
