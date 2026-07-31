package provenance

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// memStatusListStore is the allocation table in memory. Each method holds the
// lock for its whole body because each is one statement in Postgres, so the
// interleavings it admits are the ones the real store admits — the allocation
// logic under test is the production one, only the rows live elsewhere.
type memStatusListStore struct {
	mu      sync.Mutex
	entries map[string]StatusListEntry
	slots   map[StatusListEntry]string
	cursor  map[int]uint32
	size    uint32
}

func newMemStatusListStore(listID int, size uint32) *memStatusListStore {
	return &memStatusListStore{
		entries: map[string]StatusListEntry{},
		slots:   map[StatusListEntry]string{},
		cursor:  map[int]uint32{listID: 0},
		size:    size,
	}
}

// inherit records an assignment as the migration preserves it: a slot held by
// the retired hash scheme, which an allocation must step over.
func (s *memStatusListStore) inherit(subjectID string, entry StatusListEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[subjectID] = entry
	if _, taken := s.slots[entry]; !taken {
		s.slots[entry] = subjectID
	}
}

func (s *memStatusListStore) Entry(_ context.Context, subjectID string) (StatusListEntry, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[subjectID]
	return entry, ok, nil
}

func (s *memStatusListStore) ReserveIndex(_ context.Context, listID int) (uint32, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next, registered := s.cursor[listID]
	if !registered {
		return 0, false, fmt.Errorf("status list %d is not registered in status_list_cursors", listID)
	}
	if next >= s.size {
		return 0, false, nil
	}
	s.cursor[listID] = next + 1
	return next, true, nil
}

func (s *memStatusListStore) Claim(_ context.Context, subjectID string, entry StatusListEntry) (StatusListEntry, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if stored, ok := s.entries[subjectID]; ok {
		return stored, true, nil
	}
	if _, taken := s.slots[entry]; taken {
		return StatusListEntry{}, false, nil
	}
	s.entries[subjectID] = entry
	s.slots[entry] = subjectID
	return entry, true, nil
}

func newTestAllocator(size uint32) (*StatusListAllocator, *memStatusListStore) {
	store := newMemStatusListStore(DefaultListID, size)
	return &StatusListAllocator{store: store, listID: DefaultListID}, store
}

// newTestPublisher is a publisher whose entries are allocated in memory.
func newTestPublisher(serviceURL, tenantID string) *OCMWStatusListPublisher {
	allocator, _ := newTestAllocator(listSize)
	return NewOCMWStatusListPublisher(serviceURL, "did:example:issuer", tenantID, allocator)
}

// TestDistinctContractsNeverShareARevocationEntry is the defect this allocation
// exists for. The entry is the CONTRACT's revocation bit, so two contracts
// sharing one means terminating either marks the other revoked, for every
// verifier that checks its lifecycle credential.
//
// Nothing about the contract id may decide the entry. Deriving it from
// sha256(did) — as this did — puts 5000 of these ids on 4926 distinct bits: 74
// contracts inherit a stranger's revocation state, and the first pair appears
// inside the first 500.
func TestDistinctContractsNeverShareARevocationEntry(t *testing.T) {
	allocator, _ := newTestAllocator(listSize)

	seen := make(map[StatusListEntry]string, 5000)
	for i := 0; i < 5000; i++ {
		contractID := fmt.Sprintf("did:web:example.org:contracts:%d", i)
		entry, err := allocator.Allocate(context.Background(), contractID)
		require.NoError(t, err)
		if other, clash := seen[entry]; clash {
			t.Fatalf("%s and %s share status list entry %d: revoking one revokes the other", other, contractID, entry.Index)
		}
		seen[entry] = contractID
	}
}

// TestARevocationEntryIsStableForTheLifeOfTheContract pins the other half: a
// credential already issued advertises the entry, so re-asking must never move
// it. Every lifecycle credential a contract accumulates is a fresh call here.
func TestARevocationEntryIsStableForTheLifeOfTheContract(t *testing.T) {
	allocator, _ := newTestAllocator(listSize)
	const contractID = "did:web:example.org:contracts:stable"

	// Other contracts allocating in between must not shift it.
	first, err := allocator.Allocate(context.Background(), contractID)
	require.NoError(t, err)
	for i := 0; i < 10; i++ {
		_, err := allocator.Allocate(context.Background(), fmt.Sprintf("did:web:example.org:contracts:other-%d", i))
		require.NoError(t, err)
	}
	for i := 0; i < 3; i++ {
		again, err := allocator.Allocate(context.Background(), contractID)
		require.NoError(t, err)
		assert.Equal(t, first, again, "re-issuing a credential must not move the contract's revocation entry")
	}
}

// TestConcurrentAllocationForOneContractYieldsOneEntry: two lifecycle events of
// the same contract can be stamped at once, and the contract has one revocation
// bit, not one per racing writer.
func TestConcurrentAllocationForOneContractYieldsOneEntry(t *testing.T) {
	allocator, store := newTestAllocator(listSize)
	const contractID = "did:web:example.org:contracts:raced"

	const writers = 32
	results := make([]StatusListEntry, writers)
	errs := make([]error, writers)
	var start sync.WaitGroup
	var done sync.WaitGroup
	start.Add(1)
	for i := 0; i < writers; i++ {
		done.Add(1)
		go func(i int) {
			defer done.Done()
			start.Wait()
			results[i], errs[i] = allocator.Allocate(context.Background(), contractID)
		}(i)
	}
	start.Done()
	done.Wait()

	for i := range errs {
		require.NoError(t, errs[i])
		assert.Equal(t, results[0], results[i], "concurrent allocation handed out two entries for one contract")
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	assert.Len(t, store.entries, 1, "one contract must occupy exactly one row")
}

// TestConcurrentAllocationForDistinctContractsYieldsDistinctEntries: the
// exactly-once guarantee has to hold across contracts too, or the race
// reintroduces the collision by another route.
func TestConcurrentAllocationForDistinctContractsYieldsDistinctEntries(t *testing.T) {
	allocator, _ := newTestAllocator(listSize)

	const writers = 200
	results := make([]StatusListEntry, writers)
	errs := make([]error, writers)
	var start sync.WaitGroup
	var done sync.WaitGroup
	start.Add(1)
	for i := 0; i < writers; i++ {
		done.Add(1)
		go func(i int) {
			defer done.Done()
			start.Wait()
			results[i], errs[i] = allocator.Allocate(context.Background(), fmt.Sprintf("did:web:example.org:contracts:c%d", i))
		}(i)
	}
	start.Done()
	done.Wait()

	seen := make(map[StatusListEntry]int, writers)
	for i := range errs {
		require.NoError(t, errs[i])
		if other, clash := seen[results[i]]; clash {
			t.Fatalf("contracts %d and %d both got status list entry %d", other, i, results[i].Index)
		}
		seen[results[i]] = i
	}
}

// TestAllocationStepsOverEntriesInheritedFromTheHashScheme: the migration keeps
// every assignment made before this table existed, because credentials in the
// wild advertise them. A new allocation must therefore treat an inherited slot
// as taken — handing it out would create exactly the collision being removed.
func TestAllocationStepsOverEntriesInheritedFromTheHashScheme(t *testing.T) {
	allocator, store := newTestAllocator(listSize)
	store.inherit("did:web:example.org:contracts:legacy-0", StatusListEntry{ListID: DefaultListID, Index: 0})
	store.inherit("did:web:example.org:contracts:legacy-1", StatusListEntry{ListID: DefaultListID, Index: 1})

	entry, err := allocator.Allocate(context.Background(), "did:web:example.org:contracts:fresh")
	require.NoError(t, err)
	assert.Equal(t, uint32(2), entry.Index, "the first two entries are held by preserved assignments")

	inherited, err := allocator.Allocate(context.Background(), "did:web:example.org:contracts:legacy-1")
	require.NoError(t, err)
	assert.Equal(t, uint32(1), inherited.Index, "an inherited assignment must be returned unchanged, not reallocated")
}

// TestAFullStatusListFailsLoudly: 2^17 entries is finite and the service creates
// further lists only on an operator's NATS event, so there is nothing to roll
// over into automatically. Wrapping would silently hand a used bit to a second
// contract — the failure this whole change removes — so exhaustion is an error.
func TestAFullStatusListFailsLoudly(t *testing.T) {
	allocator, _ := newTestAllocator(2)

	for i := 0; i < 2; i++ {
		_, err := allocator.Allocate(context.Background(), fmt.Sprintf("did:web:example.org:contracts:%d", i))
		require.NoError(t, err)
	}

	_, err := allocator.Allocate(context.Background(), "did:web:example.org:contracts:overflow")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is full")
	assert.Contains(t, err.Error(), "STATUSLIST_LIST_ID")
}

// TestAllocationInAnUnregisteredListFails: pointing STATUSLIST_LIST_ID at a list
// nobody registered would advertise entries in a list the service does not
// serve, where the revoke POST has nothing to flip.
func TestAllocationInAnUnregisteredListFails(t *testing.T) {
	store := newMemStatusListStore(DefaultListID, listSize)
	allocator := &StatusListAllocator{store: store, listID: 7}

	_, err := allocator.Allocate(context.Background(), "did:web:example.org:contracts:elsewhere")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
}

// TestAllocationNeedsASubject guards the one input that must never be defaulted:
// an empty subject would collect every anonymous caller on one shared bit.
func TestAllocationNeedsASubject(t *testing.T) {
	allocator, _ := newTestAllocator(listSize)
	_, err := allocator.Allocate(context.Background(), "   ")
	require.Error(t, err)
}
