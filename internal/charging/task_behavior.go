package charging

import (
	"errors"
	"runtime"
)

var ErrReminderClaimed = errors.New("reminder already claimed")

type ReminderQueue struct{ claims map[string]string }

func NewReminderQueue() *ReminderQueue { return &ReminderQueue{claims: map[string]string{}} }
func (q *ReminderQueue) Claim(id, worker string) error {
	if _, ok := q.claims[id]; ok {
		return ErrReminderClaimed
	}
	runtime.Gosched()
	q.claims[id] = worker
	return nil
}
func (q *ReminderQueue) Owner(id string) string { return q.claims[id] }
