package pq

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

// recordingDriver captures the statement and arguments the repository actually
// sends, and answers with no rows.
type recordingDriver struct{ state *recordedQuery }
type recordedQuery struct {
	mu    sync.Mutex
	query string
	args  []driver.NamedValue
}
type recordingConn struct{ state *recordedQuery }
type noRows struct{}

func (d recordingDriver) Open(string) (driver.Conn, error) {
	return &recordingConn{state: d.state}, nil
}
func (c *recordingConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare is unsupported")
}
func (c *recordingConn) Close() error { return nil }
func (c *recordingConn) Begin() (driver.Tx, error) {
	return nopTx{}, nil
}
func (c *recordingConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	c.state.query = query
	c.state.args = args
	return noRows{}, nil
}

func (noRows) Columns() []string         { return []string{} }
func (noRows) Close() error              { return nil }
func (noRows) Next([]driver.Value) error { return io.EOF }

type nopTx struct{}

func (nopTx) Commit() error   { return nil }
func (nopTx) Rollback() error { return nil }

// The retry pass is serial and each entry costs a did:web resolution, a
// trust-gate call and a peer POST. An unbounded pass — the query was once a
// bare "SELECT * FROM sync_fails" — grows with every contract that can never
// ship, until one pass outlasts the interval between passes and a contract
// offered right now waits behind all of them.
func TestPendingShipRetriesAreBoundedAndBackedOff(t *testing.T) {
	state := &recordedQuery{}
	driverName := "sync-fails-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	sql.Register(driverName, recordingDriver{state: state})
	rawDB, err := sql.Open(driverName, "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = rawDB.Close() })
	db := sqlx.NewDb(rawDB, driverName)

	tx, err := db.Beginx()
	require.NoError(t, err)
	_, err = PostgresSyncRepository{}.GetPendingSyncFails(context.Background(), tx, 10*time.Second, 10*time.Minute, 25)
	require.NoError(t, err)

	state.mu.Lock()
	defer state.mu.Unlock()

	require.Contains(t, state.query, "LIMIT", "an unbounded pass starves the ships that can still succeed")
	require.Contains(t, state.query, "ORDER BY last_tried_at", "entries must take turns, not let a fixed prefix win every pass")
	require.Contains(t, state.query, "retry_count", "an entry that keeps failing must back off")

	require.Len(t, state.args, 3)
	require.EqualValues(t, 10, state.args[0].Value, "backoff base is the scheduler's own interval, in seconds")
	require.EqualValues(t, 600, state.args[1].Value, "backoff is capped so a recovered peer is still retried")
	require.EqualValues(t, 25, state.args[2].Value, "one pass is bounded")
}
