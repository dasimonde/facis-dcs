package pq

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype/componenttype"
)

func PostgresPersistEvent(ctx context.Context, tx *sqlx.Tx, component componenttype.ComponentType, eventType string, eventJSON []byte, did string) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO outbox_events 
		 (component, event_type, event_data, did, processed)
		 VALUES ($1, $2, $3, $4, FALSE)`,
		component.String(),
		eventType,
		eventJSON,
		did,
	)

	if err != nil {
		return fmt.Errorf("failed to insert event into outbox: %w", err)
	}

	return nil
}

func PostgresUpdateOutboxEvent(ctx context.Context, tx *sqlx.Tx, id int64) error {
	_, err := tx.ExecContext(ctx, `
        UPDATE outbox_events
        SET processed = TRUE, processed_at = CURRENT_TIMESTAMP
        WHERE id = $1
    `, id)

	if err != nil {
		return fmt.Errorf("could not mark event %d as processed: %w", id, err)
	}

	return nil
}

func PostgresMarkOutboxEventPublished(ctx context.Context, tx *sqlx.Tx, id int64) error {
	_, err := tx.ExecContext(ctx, `
        UPDATE outbox_events
        SET published = TRUE, published_at = CURRENT_TIMESTAMP
        WHERE id = $1
    `, id)

	if err != nil {
		return fmt.Errorf("could not mark event %d as published: %w", id, err)
	}

	return nil
}

// PostgresRecordOutboxAnchorFailure counts one failed anchoring attempt and
// stores its error. When the attempts reach maxAttempts the event is
// dead-lettered: the anchoring loop stops selecting it, so a permanently
// unanchorable event neither spins forever nor vanishes. It reports whether
// this call was the one that dead-lettered the event.
func PostgresRecordOutboxAnchorFailure(ctx context.Context, tx *sqlx.Tx, id int64, cause string, maxAttempts int) (bool, error) {
	var deadLettered bool
	err := tx.QueryRowContext(ctx, `
        UPDATE outbox_events
        SET anchor_attempts = anchor_attempts + 1,
            anchor_error = $2,
            dead_lettered_at = CASE
                WHEN anchor_attempts + 1 >= $3 THEN CURRENT_TIMESTAMP
                ELSE dead_lettered_at
            END
        WHERE id = $1
        RETURNING dead_lettered_at IS NOT NULL
    `, id, cause, maxAttempts).Scan(&deadLettered)
	if err != nil {
		return false, fmt.Errorf("could not record anchoring failure for event %d: %w", id, err)
	}
	return deadLettered, nil
}
