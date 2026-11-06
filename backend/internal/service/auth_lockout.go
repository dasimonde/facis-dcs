package service

import (
	"context"
	"errors"
	"time"

	genauth "digital-contracting-service/gen/auth"
	authevents "digital-contracting-service/internal/auth/event"
	"digital-contracting-service/internal/base/datatype/componenttype"
	baseevent "digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/middleware"
)

// Lockouts lists the active lockouts for the System Administrator
// (FR-UC-01-4, NFR-SEC-11).
func (s *authSvc) Lockouts(ctx context.Context, _ *genauth.LockoutsPayload) ([]*genauth.LockoutInfo, error) {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	locks, err := s.LockRepo.ListActive(ctx, tx)
	if err != nil {
		return nil, err
	}
	out := make([]*genauth.LockoutInfo, 0, len(locks))
	for _, l := range locks {
		out = append(out, &genauth.LockoutInfo{Key: l.Key, LockedUntil: l.LockedUntil.UTC().Format(time.RFC3339)})
	}
	return out, nil
}

// Unlock ends an active lockout now and records who ended it in the audit
// trail, in one transaction.
func (s *authSvc) Unlock(ctx context.Context, p *genauth.UnlockPayload) error {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	lifted, err := s.LockRepo.ClearLockout(ctx, tx, p.Key)
	if err != nil {
		return err
	}
	if !lifted {
		return genauth.MakeNotFound(errors.New("no active lockout for this key"))
	}
	if err := baseevent.Create(ctx, tx, authevents.LockoutLiftedEvent{
		LockoutKey:          p.Key,
		ActorDID:            middleware.GetHolderDID(ctx),
		ActorParticipantDID: middleware.GetParticipantID(ctx),
		OccurredAt:          time.Now().UTC(),
	}, componenttype.System); err != nil {
		return err
	}
	return tx.Commit()
}
