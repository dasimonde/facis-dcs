-- The Semantic Hub serves the ontology itself as a versioned schema kind
-- (GET /semantic/ontology/{name}); admit it to the kind check.
ALTER TABLE semantic_schemas
    DROP CONSTRAINT semantic_schemas_kind_check;

ALTER TABLE semantic_schemas
    ADD CONSTRAINT semantic_schemas_kind_check
    CHECK (kind IN ('context', 'shapes', 'profile', 'ontology'));
