package charging

import (
	"errors"
	"sync"
)

var ErrReminderClaimed = errors.New("reminder already claimed")

type ReminderQueue struct {
	claims map[string]string
	mu     sync.Mutex
}

func NewReminderQueue() *ReminderQueue { return &ReminderQueue{claims: map[string]string{}} }
func (q *ReminderQueue) Claim(id, worker string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if _, ok := q.claims[id]; ok {
		return ErrReminderClaimed
	}
	q.claims[id] = worker
	return nil
}
func (q *ReminderQueue) Owner(id string) string {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.claims[id]
}
