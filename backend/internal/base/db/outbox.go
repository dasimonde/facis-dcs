package db

import (
	"context"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/db/pq"
)

func PersistEvent(ctx context.Context, tx *sqlx.Tx, component componenttype.ComponentType, eventType string, eventJSON []byte, did string) error {
	return pq.PostgresPersistEvent(ctx, tx, component, eventType, eventJSON, did)
}

func UpdateOutboxEvent(ctx context.Context, tx *sqlx.Tx, id int64) error {
	return pq.PostgresUpdateOutboxEvent(ctx, tx, id)
}

func MarkOutboxEventPublished(ctx context.Context, tx *sqlx.Tx, id int64) error {
	return pq.PostgresMarkOutboxEventPublished(ctx, tx, id)
}

// RecordOutboxAnchorFailure counts a failed anchoring attempt and dead-letters
// the event once maxAttempts is reached. Returns true when the event is now
// dead-lettered.
func RecordOutboxAnchorFailure(ctx context.Context, tx *sqlx.Tx, id int64, cause string, maxAttempts int) (bool, error) {
	return pq.PostgresRecordOutboxAnchorFailure(ctx, tx, id, cause, maxAttempts)
}
