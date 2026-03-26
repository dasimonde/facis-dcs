CREATE TYPE delivery_type AS ENUM ('LOCAL', 'EXTERNAL');


CREATE TABLE IF NOT EXISTS outbox_events (
    id BIGSERIAL PRIMARY KEY,

    component VARCHAR(64) NOT NULL,
    delivery_type delivery_type NOT NULL DEFAULT 'LOCAL',

    event_type VARCHAR(64) NOT NULL,
    event_data JSONB NOT NULL,

    did VARCHAR(255),

    processed BOOLEAN DEFAULT FALSE,
    processed_at TIMESTAMP,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);


CREATE INDEX idx_outbox_events_processed 
    ON outbox_events(processed, created_at)
    WHERE processed = FALSE;


CREATE INDEX idx_outbox_events_created_at 
    ON outbox_events(created_at);


CREATE INDEX idx_outbox_events_did
    ON outbox_events(did);


CREATE INDEX idx_outbox_events_type
    ON outbox_events(event_type);


CREATE OR REPLACE FUNCTION add_outbox_event(
    p_event_type VARCHAR,
    p_event_data JSONB,
    p_did VARCHAR
) RETURNS BIGINT AS $$
DECLARE
    v_event_id BIGINT;
BEGIN
    INSERT INTO outbox_events (event_type, event_data, did, processed)
    VALUES (p_event_type, p_event_data, p_did, FALSE)
    RETURNING id INTO v_event_id;
    
    RETURN v_event_id;
END;
$$ LANGUAGE plpgsql;


CREATE OR REPLACE FUNCTION mark_event_processed(p_event_id BIGINT)
RETURNS BOOLEAN AS $$
BEGIN
    UPDATE outbox_events 
    SET processed = TRUE, processed_at = CURRENT_TIMESTAMP
    WHERE id = p_event_id;
    
    RETURN FOUND;
END;
$$ LANGUAGE plpgsql;


CREATE OR REPLACE FUNCTION cleanup_old_events(p_retention_period INTERVAL DEFAULT '7 days')
RETURNS INTEGER AS $$
DECLARE
    v_deleted_count INTEGER;
BEGIN
    DELETE FROM outbox_events
    WHERE processed = TRUE 
      AND processed_at < CURRENT_TIMESTAMP - p_retention_period;
    
    GET DIAGNOSTICS v_deleted_count = ROW_COUNT;
    RETURN v_deleted_count;
END;
$$ LANGUAGE plpgsql;


CREATE OR REPLACE VIEW v_unprocessed_events AS
SELECT 
    id,
    event_type,
    event_data,
    did,
    created_at,
    CURRENT_TIMESTAMP - created_at AS age
FROM outbox_events
WHERE processed = FALSE
ORDER BY created_at ASC;


CREATE OR REPLACE VIEW v_outbox_stats AS
SELECT 
    event_type,
    COUNT(*) FILTER (WHERE processed = FALSE) AS unprocessed_count,
    COUNT(*) FILTER (WHERE processed = TRUE) AS processed_count,
    COUNT(*) AS total_count,
    MAX(created_at) AS latest_event
FROM outbox_events
GROUP BY event_type

ORDER BY unprocessed_count DESC;
