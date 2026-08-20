package charging

import (
	"errors"
	"sync"
)

// ErrReminderClaimed is returned when an overdue record is already being
// processed by another worker, so a reminder must not be sent again.
var ErrReminderClaimed = errors.New("reminder already claimed")

// ReminderQueue serializes ownership of overdue records across workers so a
// single record is handled by at most one worker at a time. A successful claim
// is held until Release is called; on processing failure the owning worker
// releases the record so the next scan can reclaim and retry it.
type ReminderQueue struct {
	mu     sync.Mutex
	claims map[string]string
}

func NewReminderQueue() *ReminderQueue { return &ReminderQueue{claims: map[string]string{}} }

// Claim atomically reserves id for worker. It returns ErrReminderClaimed when
// id is already owned, so concurrent claims for the same record yield exactly
// one winner and the rest are rejected without sending a duplicate reminder.
func (q *ReminderQueue) Claim(id, worker string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if _, ok := q.claims[id]; ok {
		return ErrReminderClaimed
	}
	q.claims[id] = worker
	return nil
}

// Release frees a previously claimed id so it can be retried after a failed
// processing attempt. Releasing an unclaimed id is a no-op.
func (q *ReminderQueue) Release(id string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.claims, id)
}

// Owner returns the worker currently holding id, or "" when it is unclaimed.
func (q *ReminderQueue) Owner(id string) string {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.claims[id]
}
