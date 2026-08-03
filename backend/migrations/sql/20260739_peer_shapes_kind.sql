-- ADR-8: a contract shipped by a peer pins the SHACL graphs it was authored
-- against, and the graphs travel with it. They are stored under their own
-- kind so no query that answers "what does THIS instance publish" can ever
-- return one: Seed's latest-version probe, ResolveEffectiveBundle, the
-- active-library index, and the /semantic/schema/* admin endpoints are all
-- scoped to kind='shapes', which is what keeps a peer from writing or
-- shadowing this instance's own canonical entries.
ALTER TABLE semantic_schemas
    DROP CONSTRAINT semantic_schemas_kind_check;

ALTER TABLE semantic_schemas
    ADD CONSTRAINT semantic_schemas_kind_check
    CHECK (kind IN ('context', 'shapes', 'profile', 'ontology', 'peer-shapes'));
