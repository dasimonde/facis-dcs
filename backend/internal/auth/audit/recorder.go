package audit

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	authevents "digital-contracting-service/internal/auth/event"
	"digital-contracting-service/internal/auth/oid4vp"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
)

// Recorder persists OID4VP presentation outcomes to the outbox audit trail.
type Recorder struct {
	DB *sqlx.DB
}

func (r *Recorder) RecordPresentationAudit(ctx context.Context, evt oid4vp.PresentationAuditEvent) error {
	if r == nil || r.DB == nil {
		return fmt.Errorf("auth audit recorder is not configured")
	}

	state := strings.TrimSpace(evt.PresentationState)
	if state == "" {
		return fmt.Errorf("presentation_state is required for audit")
	}

	tx, err := r.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("start auth audit transaction: %w", err)
	}

	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			_ = err
		}
	}(tx)

	occurredAt := time.Now().UTC()

	if evt.Success {
		out := authevents.PresentationSucceededEvent{
			PresentationState: state,
			SubjectDID:        strings.TrimSpace(evt.SubjectDID),
			ParticipantDID:    strings.TrimSpace(evt.ParticipantDID),
			Roles:             append([]string(nil), evt.Roles...),
			OccurredAt:        occurredAt,
		}
		err = event.Create(ctx, tx, out, componenttype.System)
	} else {
		out := authevents.PresentationFailedEvent{
			PresentationState: state,
			SubjectDID:        strings.TrimSpace(evt.SubjectDID),
			ParticipantDID:    strings.TrimSpace(evt.ParticipantDID),
			Roles:             append([]string(nil), evt.Roles...),
			ErrorMessage:      strings.TrimSpace(evt.ErrorMessage),
			OccurredAt:        occurredAt,
		}
		err = event.Create(ctx, tx, out, componenttype.System)
	}

	if err != nil {
		return fmt.Errorf("persist auth presentation audit event: %w", err)
	}

	err = tx.Commit()

	if err != nil {
		return fmt.Errorf("commit auth presentation audit event: %w", err)
	}

	return nil
}
