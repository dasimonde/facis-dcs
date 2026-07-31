-- DCS-FR-TR-03 (Semantic Hub): versioned repository for the machine-readable
-- schemas the DCS produces documents against — JSON-LD contexts, SHACL
-- shapes, and validation profiles. Every produced JSON-LD artifact anchors
-- its schemaRefs to hub-served, versioned URLs (GET /semantic/context/...,
-- /semantic/shapes/...), and the templating/contracting normalization layer
-- enforces the ACTIVE context's ontology IRIs (UC-02-08: versioning and
-- rollback are first-class).
CREATE TABLE IF NOT EXISTS semantic_schemas
(
    name       VARCHAR(255) NOT NULL CHECK (name <> ''),
    version    INT          NOT NULL CHECK (version > 0),
    kind       VARCHAR(32)  NOT NULL CHECK (kind IN ('context', 'shapes', 'profile')),
    media_type VARCHAR(128) NOT NULL,
    content    TEXT         NOT NULL CHECK (content <> ''),
    active     BOOLEAN      NOT NULL DEFAULT FALSE,
    created_by VARCHAR(255) NOT NULL,
    created_at TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT pk_semantic_schemas PRIMARY KEY (name, kind, version)
);

-- Exactly one active version per (name, kind).
CREATE UNIQUE INDEX idx_semantic_schemas_one_active
    ON semantic_schemas (name, kind) WHERE active;
