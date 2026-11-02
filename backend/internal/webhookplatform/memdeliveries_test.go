package webhookplatform

import (
	"context"
	"sync"
	"time"
)

// memDeliveries is the in-memory Deliveries the unit tests use.
type memDeliveries struct {
	mu         sync.Mutex
	pending    map[string]PendingCallback
	deliveries []Delivery
	due        map[int64]time.Time
}

func newMemDeliveries() *memDeliveries {
	return &memDeliveries{pending: make(map[string]PendingCallback), due: make(map[int64]time.Time)}
}

func (s *memDeliveries) RecordDispatch(_ context.Context, p PendingCallback, deliveries []Delivery, retryAt time.Time) ([]int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.pending[p.EventID]; ok {
		return nil, nil
	}
	s.pending[p.EventID] = p
	ids := make([]int64, 0, len(deliveries))
	for _, d := range deliveries {
		ids = append(ids, s.add(d, retryAt))
	}
	return ids, nil
}

// queue records one delivery due at retryAt, as a stopped replica leaves it.
func (s *memDeliveries) queue(d Delivery, retryAt time.Time) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.add(d, retryAt)
}

func (s *memDeliveries) add(d Delivery, retryAt time.Time) int64 {
	d.ID = int64(len(s.deliveries) + 1)
	s.deliveries = append(s.deliveries, d)
	s.due[d.ID] = retryAt
	return d.ID
}

func (s *memDeliveries) RecordAttempt(_ context.Context, id int64, statusCode int, errText string, next time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := &s.deliveries[id-1]
	d.StatusCode, d.Error, d.Attempts, d.DeliveredAt = statusCode, errText, d.Attempts+1, time.Now().UTC()
	if next.IsZero() {
		delete(s.due, id)
		d.Payload = nil
	} else {
		s.due[id] = next
	}
	return nil
}

func (s *memDeliveries) DueDeliveries(_ context.Context, now time.Time, limit int) ([]Delivery, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Delivery{}
	for _, d := range s.deliveries {
		if at, ok := s.due[d.ID]; ok && !at.After(now) && len(out) < limit {
			out = append(out, d)
		}
	}
	return out, nil
}

func (s *memDeliveries) ResolvePending(_ context.Context, eventID, ownerDID string) (PendingCallback, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	resolved := false
	for i := range s.deliveries {
		if d := &s.deliveries[i]; d.EventID == eventID && d.OwnerDID == ownerDID && !d.Acknowledged {
			d.Acknowledged, resolved = true, true
		}
	}
	return s.pending[eventID], resolved, nil
}

func (s *memDeliveries) ListDeliveries(_ context.Context, ownerDID string) ([]Delivery, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Delivery{}
	for _, d := range s.deliveries {
		if d.OwnerDID == ownerDID {
			out = append(out, d)
		}
	}
	return out, nil
}
