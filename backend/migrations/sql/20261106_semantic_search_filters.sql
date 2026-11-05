CREATE TABLE semantic_search_filter_config (
    singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
    revision BIGINT NOT NULL,
    filters JSONB NOT NULL
);
