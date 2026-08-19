package httpapi

import (
	"chargeguard/internal/charging"
	"errors"
	"net/http"
)

var reminderQueue = charging.NewReminderQueue()

func ResetTaskHTTPState() { reminderQueue = charging.NewReminderQueue() }
func TaskHTTPHandler(w http.ResponseWriter, r *http.Request) {
	id, worker := r.URL.Query().Get("id"), r.URL.Query().Get("worker")
	err := reminderQueue.Claim(id, worker)
	if errors.Is(err, charging.ErrReminderClaimed) {
		http.Error(w, "already claimed", http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, "internal error", 500)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
