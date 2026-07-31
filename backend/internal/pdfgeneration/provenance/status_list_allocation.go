package provenance

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// StatusListEntry is one subject's position in one status list: the bit that
// says whether that contract (or template) is still in force.
type StatusListEntry struct {
	ListID int
	Index  uint32
}

// statusListStore is the persistence an allocation needs, expressed as the three
// steps that have to be atomic on their own. The Postgres implementation is
// below; tests substitute an in-memory one with the same per-call atomicity, so
// the allocation logic under test is the one that runs in production.
type statusListStore interface {
	// Entry returns the entry already recorded for subjectID, if any.
	Entry(ctx context.Context, subjectID string) (StatusListEntry, bool, error)

	// ReserveIndex advances the list's cursor and returns the index it moved
	// past. ok is false when the list is full; an unregistered list is an error.
	ReserveIndex(ctx context.Context, listID int) (index uint32, ok bool, err error)

	// Claim records subjectID at entry. claimed is false when the slot is
	// already held by someone else, in which case nothing was written. When
	// subjectID was recorded concurrently, the entry that won is returned with
	// claimed true — an allocation happens once per subject, never twice.
	Claim(ctx context.Context, subjectID string, entry StatusListEntry) (stored StatusListEntry, claimed bool, err error)
}

// maxClaimAttempts bounds how many occupied slots an allocation walks past
// before giving up. Only indices held by assignments inherited from the retired
// hash scheme are ever occupied ahead of the cursor, and they are as sparse as
// the deployment's contract count over the list, so a run this long means the
// cursor and the table disagree rather than that the next slot is busy.
const maxClaimAttempts = 64

// StatusListAllocator assigns each subject the status list entry it keeps for
// life. It replaces a hash of the subject id: a hash needs no table but lets two
// subjects share one bit, and since that bit is the contract's revocation state,
// sharing it means revoking one contract revokes the other.
//
// Allocation is exactly once per subject. Every later call — every re-issued
// credential, and the revocation that eventually flips the bit — reads back the
// same entry, which is what keeps the index a credential advertises and the
// index a revocation POSTs identical.
type StatusListAllocator struct {
	store  statusListStore
	listID int
}

// NewPostgresStatusListAllocator returns the allocator backed by the
// status_list_entries / status_list_cursors tables (migration 20260734).
// listID must name a list registered in status_list_cursors AND served by the
// statuslist-service; entries in a list the service does not serve advertise a
// URI that does not resolve.
func NewPostgresStatusListAllocator(db *sqlx.DB, listID int) *StatusListAllocator {
	return &StatusListAllocator{store: &postgresStatusListStore{db: db}, listID: listID}
}

// Allocate returns subjectID's entry, assigning one the first time it is asked.
func (a *StatusListAllocator) Allocate(ctx context.Context, subjectID string) (StatusListEntry, error) {
	subjectID = strings.TrimSpace(subjectID)
	if subjectID == "" {
		return StatusListEntry{}, errors.New("status list allocation needs a subject id")
	}

	entry, found, err := a.store.Entry(ctx, subjectID)
	if err != nil {
		return StatusListEntry{}, fmt.Errorf("read status list entry of %s: %w", subjectID, err)
	}
	if found {
		return entry, nil
	}

	for attempt := 0; attempt < maxClaimAttempts; attempt++ {
		index, ok, err := a.store.ReserveIndex(ctx, a.listID)
		if err != nil {
			return StatusListEntry{}, fmt.Errorf("reserve an index in status list %d for %s: %w", a.listID, subjectID, err)
		}
		if !ok {
			return StatusListEntry{}, fmt.Errorf(
				"status list %d is full: allocate a new list in the statuslist-service, register it in status_list_cursors and set STATUSLIST_LIST_ID to it (%s has no revocation entry until then)",
				a.listID, subjectID)
		}

		stored, claimed, err := a.store.Claim(ctx, subjectID, StatusListEntry{ListID: a.listID, Index: index})
		if err != nil {
			return StatusListEntry{}, fmt.Errorf("claim status list %d entry %d for %s: %w", a.listID, index, subjectID, err)
		}
		if claimed {
			return stored, nil
		}
		// The slot belongs to an assignment inherited from the hash scheme.
		// Leave it where it is and take the next one.
	}
	return StatusListEntry{}, fmt.Errorf(
		"no free entry in status list %d for %s after %d attempts: the cursor is behind the entries already recorded",
		a.listID, subjectID, maxClaimAttempts)
}

// postgresStatusListStore is the allocation table. Each method is a single
// statement, so each is atomic without an enclosing transaction — deliberately:
// an allocation must be durable the moment it is handed out, because the
// credential that advertises it may be published before whatever transaction
// the caller happens to be inside commits.
type postgresStatusListStore struct {
	db *sqlx.DB
}

func (s *postgresStatusListStore) Entry(ctx context.Context, subjectID string) (StatusListEntry, bool, error) {
	var row struct {
		ListID int   `db:"list_id"`
		Index  int64 `db:"entry_index"`
	}
	err := s.db.GetContext(ctx, &row,
		`SELECT list_id, entry_index FROM status_list_entries WHERE subject_id = $1`, subjectID)
	if errors.Is(err, sql.ErrNoRows) {
		return StatusListEntry{}, false, nil
	}
	if err != nil {
		return StatusListEntry{}, false, err
	}
	return StatusListEntry{ListID: row.ListID, Index: uint32(row.Index)}, true, nil
}

func (s *postgresStatusListStore) ReserveIndex(ctx context.Context, listID int) (uint32, bool, error) {
	var index int64
	err := s.db.GetContext(ctx, &index,
		`UPDATE status_list_cursors
            SET next_index = next_index + 1
          WHERE list_id = $1 AND next_index < list_size
      RETURNING next_index - 1`, listID)
	if errors.Is(err, sql.ErrNoRows) {
		// Either the list is full or it was never registered, and those are
		// different problems: one needs a new list, the other a configuration fix.
		var registered bool
		if err := s.db.GetContext(ctx, &registered,
			`SELECT EXISTS (SELECT 1 FROM status_list_cursors WHERE list_id = $1)`, listID); err != nil {
			return 0, false, err
		}
		if !registered {
			return 0, false, fmt.Errorf("status list %d is not registered in status_list_cursors", listID)
		}
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return uint32(index), true, nil
}

func (s *postgresStatusListStore) Claim(ctx context.Context, subjectID string, entry StatusListEntry) (StatusListEntry, bool, error) {
	var row struct {
		ListID int   `db:"list_id"`
		Index  int64 `db:"entry_index"`
	}
	// The NOT EXISTS keeps the allocation off any slot already recorded,
	// including the inherited ones no unique index can cover. Two allocations
	// racing for the same free slot both pass it; the loser is caught by
	// idx_status_list_entries_allocated_slot and retries.
	err := s.db.GetContext(ctx, &row,
		`INSERT INTO status_list_entries (subject_id, list_id, entry_index, origin)
         SELECT $1, $2, $3, 'allocated'
          WHERE NOT EXISTS (SELECT 1 FROM status_list_entries WHERE list_id = $2 AND entry_index = $3)
     ON CONFLICT (subject_id) DO NOTHING
       RETURNING list_id, entry_index`,
		subjectID, entry.ListID, int64(entry.Index))
	switch {
	case err == nil:
		return StatusListEntry{ListID: row.ListID, Index: uint32(row.Index)}, true, nil
	case isUniqueViolation(err):
		// Another allocation took this slot between the check and the insert.
		return StatusListEntry{}, false, nil
	case !errors.Is(err, sql.ErrNoRows):
		return StatusListEntry{}, false, err
	}

	// Nothing was inserted: either the slot is taken, or this subject was
	// allocated for concurrently and that allocation is the one that counts.
	stored, found, err := s.Entry(ctx, subjectID)
	if err != nil {
		return StatusListEntry{}, false, err
	}
	if found {
		return stored, true, nil
	}
	return StatusListEntry{}, false, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pq.Error
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
