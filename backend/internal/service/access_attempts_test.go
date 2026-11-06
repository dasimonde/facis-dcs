package service

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"digital-contracting-service/internal/auth/db"
)

func TestAccessAttemptCursorValidation(t *testing.T) {
	row := db.AccessAttempt{ID: 42, AttemptedAt: time.Date(2026, 10, 15, 12, 0, 0, 123000, time.UTC)}
	raw := encodeAccessCursor(row)
	cursor, err := decodeAccessCursor(raw)
	if err != nil || cursor.ID != row.ID || !cursor.AttemptedAt.Equal(row.AttemptedAt) {
		t.Fatalf("cursor = %+v, %v", cursor, err)
	}
	for _, invalid := range []string{"", "!", strings.Repeat("a", 257),
		base64.RawURLEncoding.EncodeToString([]byte("bad|42")),
		base64.RawURLEncoding.EncodeToString([]byte("2026-10-15T12:00:00Z|0")),
		base64.RawURLEncoding.EncodeToString([]byte("2026-10-15T12:00:00Z|42|extra")),
		raw + "\n",
	} {
		if _, err := decodeAccessCursor(invalid); err == nil {
			t.Errorf("accepted invalid cursor %q", invalid)
		}
	}
}

func TestAccessAttemptsPageUsesLastReturnedRowAndEmptyRoles(t *testing.T) {
	at := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	rows := []db.AccessAttempt{
		{ID: 3, AttemptedAt: at, Roles: `["Auditor"]`, Outcome: db.AccessOutcomeSuccess},
		{ID: 2, AttemptedAt: at, Roles: `null`, Outcome: db.AccessOutcomeFailed},
		{ID: 1, AttemptedAt: at, Roles: `[]`, Outcome: db.AccessOutcomeDenied},
	}
	page, err := accessAttemptsPage(rows, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Attempts) != 2 || page.NextCursor == nil {
		t.Fatalf("page = %+v", page)
	}
	cursor, err := decodeAccessCursor(*page.NextCursor)
	if err != nil || cursor.ID != 2 {
		t.Fatalf("cursor = %+v, %v", cursor, err)
	}
	if page.Attempts[1].Roles == nil {
		t.Fatal("roles must be an array")
	}
	last, err := accessAttemptsPage(rows[2:], 2)
	if err != nil || last.NextCursor != nil {
		t.Fatalf("last page = %+v, %v", last, err)
	}
	empty, err := accessAttemptsPage(nil, 2)
	if err != nil || empty.Attempts == nil || len(empty.Attempts) != 0 {
		t.Fatalf("empty page = %+v, %v", empty, err)
	}
}
