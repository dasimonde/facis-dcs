// Package outboxtest provides a database fake for tests that check which audit
// events a handler commits. It accepts every statement and keeps the outbox
// inserts of committed transactions only.
package outboxtest

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"
)

// Event is one committed outbox row.
type Event struct {
	Type string
	Data map[string]any
}

// Recorder holds the outbox rows of committed transactions.
type Recorder struct {
	mu        sync.Mutex
	committed []Event
}

// Committed returns the outbox rows committed so far.
func (r *Recorder) Committed() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Event(nil), r.committed...)
}

// Open returns a *sqlx.DB backed by the fake and its recorder. Queries return
// no rows; the repositories under test are expected to be fakes.
func Open(t *testing.T) (*sqlx.DB, *Recorder) {
	t.Helper()
	rec := &Recorder{}
	raw := sql.OpenDB(connector{rec: rec})
	t.Cleanup(func() { _ = raw.Close() })
	return sqlx.NewDb(raw, "postgres"), rec
}

type connector struct{ rec *Recorder }

func (c connector) Connect(context.Context) (driver.Conn, error) { return &conn{rec: c.rec}, nil }
func (c connector) Driver() driver.Driver                        { return nil }

type conn struct {
	rec     *Recorder
	pending []Event
}

func (c *conn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("outboxtest: prepared statements are not supported")
}
func (c *conn) Close() error              { return nil }
func (c *conn) Begin() (driver.Tx, error) { c.pending = nil; return tx{c}, nil }

func (c *conn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if strings.Contains(query, "INSERT INTO outbox_events") {
		evt := Event{Type: args[1].Value.(string)}
		if err := json.Unmarshal(args[2].Value.([]byte), &evt.Data); err != nil {
			return nil, err
		}
		c.pending = append(c.pending, evt)
	}
	return driver.RowsAffected(1), nil
}

func (c *conn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return emptyRows{}, nil
}

type tx struct{ c *conn }

func (t tx) Commit() error {
	t.c.rec.mu.Lock()
	defer t.c.rec.mu.Unlock()
	t.c.rec.committed = append(t.c.rec.committed, t.c.pending...)
	t.c.pending = nil
	return nil
}

func (t tx) Rollback() error { t.c.pending = nil; return nil }

type emptyRows struct{}

func (emptyRows) Columns() []string         { return nil }
func (emptyRows) Close() error              { return nil }
func (emptyRows) Next([]driver.Value) error { return io.EOF }
