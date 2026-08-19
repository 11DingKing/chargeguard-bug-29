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
	if id == "" || worker == "" {
		http.Error(w, "claim identity required", http.StatusBadRequest)
		return
	}
	err := reminderQueue.Claim(id, worker)
	if errors.Is(err, charging.ErrReminderClaimed) {
		w.Header().Set("X-Claim-Owner", reminderQueue.Owner(id))
		http.Error(w, "already claimed", http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, "internal error", 500)
		return
	}
	w.Header().Set("X-Claim-Owner", worker)
	w.WriteHeader(http.StatusNoContent)
}
