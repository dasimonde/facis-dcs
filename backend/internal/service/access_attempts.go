package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	goa "goa.design/goa/v3/pkg"

	pac "digital-contracting-service/gen/process_audit_and_compliance"
	"digital-contracting-service/internal/auth/db"
)

func (s *processAuditAndCompliancesrvc) AccessAttempts(ctx context.Context, p *pac.AccessAttemptsPayload) (*pac.PACAccessAttempts, error) {
	if strings.TrimSpace(p.Justification) == "" || p.Limit < 1 || p.Limit > 100 {
		return nil, goa.PermanentError("bad_request", "justification and a limit between 1 and 100 are required")
	}
	var before *db.AccessAttemptCursor
	if p.Cursor != nil {
		var err error
		before, err = decodeAccessCursor(*p.Cursor)
		if err != nil {
			return nil, goa.PermanentError("bad_request", "invalid access-attempt cursor")
		}
	}
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, goa.PermanentError("internal_error", "cannot read access attempts")
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := s.AAttemptRepo.ReadPage(ctx, tx, before, p.Limit+1)
	if err != nil {
		return nil, goa.PermanentError("internal_error", "cannot read access attempts")
	}
	return accessAttemptsPage(rows, p.Limit)
}

func accessAttemptsPage(rows []db.AccessAttempt, limit int) (*pac.PACAccessAttempts, error) {
	result := &pac.PACAccessAttempts{Attempts: make([]*pac.PACAccessAttempt, 0, min(len(rows), limit))}
	if len(rows) > limit {
		cursor := encodeAccessCursor(rows[limit-1])
		result.NextCursor = &cursor
		rows = rows[:limit]
	}
	for _, row := range rows {
		roles := []string{}
		if err := json.Unmarshal([]byte(row.Roles), &roles); err != nil {
			return nil, goa.PermanentError("internal_error", "cannot read access-attempt roles")
		}
		if roles == nil {
			roles = []string{}
		}
		result.Attempts = append(result.Attempts, &pac.PACAccessAttempt{
			ID: row.ID, AttemptedAt: row.AttemptedAt.UTC().Format(time.RFC3339Nano),
			AttemptBy: row.AttemptBy, Roles: roles, Service: row.Service, Method: row.Method,
			Scope: &row.Scope, Did: row.DID, Justification: row.Justification, Outcome: row.Outcome,
		})
	}
	return result, nil
}

func encodeAccessCursor(row db.AccessAttempt) string {
	return base64.RawURLEncoding.EncodeToString([]byte(row.AttemptedAt.UTC().Format(time.RFC3339Nano) + "|" + strconv.FormatInt(row.ID, 10)))
}

func decodeAccessCursor(raw string) (*db.AccessAttemptCursor, error) {
	if len(raw) == 0 || len(raw) > 256 {
		return nil, fmt.Errorf("invalid cursor length")
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(raw)
	if err != nil {
		return nil, err
	}
	timestamp, id, ok := strings.Cut(string(decoded), "|")
	if !ok {
		return nil, fmt.Errorf("invalid cursor fields")
	}
	at, err := time.Parse(time.RFC3339Nano, timestamp)
	if err != nil || at.IsZero() {
		return nil, fmt.Errorf("invalid cursor timestamp")
	}
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n < 1 {
		return nil, fmt.Errorf("invalid cursor id")
	}
	cursor := &db.AccessAttemptCursor{AttemptedAt: at.UTC(), ID: n}
	if encodeAccessCursor(db.AccessAttempt{AttemptedAt: cursor.AttemptedAt, ID: n}) != raw {
		return nil, fmt.Errorf("noncanonical cursor")
	}
	return cursor, nil
}
