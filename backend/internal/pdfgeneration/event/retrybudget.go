package event

import (
	"strings"
	"sync"
	"time"
)

// retryBudget paces and bounds the retry sweep's regeneration attempts per
// entity: how many have failed, and the earliest time the next one may run.
// The work list is the oldest fixed-size window of entities whose stored PDF is
// not current, so without a budget an entity that can never render (an
// unrenderable payload, a permanently rejected document) is re-attempted on
// every tick for the lifetime of the process and keeps every recoverable
// failure behind it out of the window. Attempts back off from the sweep interval and stop after
// regenerationRetryAttempts; an exhausted entity is then excluded from the work
// list query itself, which is what actually frees its slot.
//
// The state is per process and deliberately not persisted: a restart is the
// operator-visible retry, and the entity is picked up again from its own
// committed state.
type retryBudget struct {
	mu       sync.Mutex
	interval time.Duration
	attempts map[string]int
	nextAt   map[string]time.Time
}

func budgetKey(kind, did string) string {
	return kind + "\x00" + did
}

// pace records the sweep interval the backoff is derived from.
func (b *retryBudget) pace(interval time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.interval = interval
}

// ready reports whether the entity may be attempted now: its backoff has
// elapsed and it has attempts left.
func (b *retryBudget) ready(kind, did string, now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	key := budgetKey(kind, did)
	if b.attempts[key] >= regenerationRetryAttempts {
		return false
	}
	next, waiting := b.nextAt[key]
	return !waiting || !now.Before(next)
}

// failed records a failed attempt and returns how many have now failed.
func (b *retryBudget) failed(kind, did string, now time.Time) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.attempts == nil {
		b.attempts = map[string]int{}
		b.nextAt = map[string]time.Time{}
	}
	key := budgetKey(kind, did)
	b.attempts[key]++
	b.nextAt[key] = now.Add(b.backoff(b.attempts[key]))
	return b.attempts[key]
}

// succeeded clears the entity's budget: the next failure starts over.
func (b *retryBudget) succeeded(kind, did string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	key := budgetKey(kind, did)
	delete(b.attempts, key)
	delete(b.nextAt, key)
}

// exhausted returns the DIDs of the given kind that have used up their
// attempts — the caller excludes them from the work-list query so they stop
// occupying the batch.
func (b *retryBudget) exhausted(kind string) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	prefix := kind + "\x00"
	var dids []string
	for key, attempts := range b.attempts {
		if attempts >= regenerationRetryAttempts && strings.HasPrefix(key, prefix) {
			dids = append(dids, strings.TrimPrefix(key, prefix))
		}
	}
	return dids
}

// backoff doubles the sweep interval per failed attempt, up to the cap. Called
// with b.mu held.
func (b *retryBudget) backoff(attempts int) time.Duration {
	delay := b.interval
	for i := 1; i < attempts; i++ {
		if delay >= maxRegenerationRetryBackoff {
			break
		}
		delay *= 2
	}
	if delay > maxRegenerationRetryBackoff {
		return maxRegenerationRetryBackoff
	}
	return delay
}
