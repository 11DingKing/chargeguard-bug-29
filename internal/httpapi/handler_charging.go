package httpapi

import (
	"chargeguard/internal/charging"
	"encoding/json"
	"errors"
	"github.com/go-chi/chi/v5"
	"net/http"
)

func (s *Server) chargingToken(r *http.Request) string {
	token := r.Header.Get("Authorization")
	if len(token) >= 7 && token[:7] == "Bearer " {
		return token[7:]
	}
	return token
}
func (s *Server) chargingError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, charging.ErrNotFound) {
		status = http.StatusNotFound
	}
	if err.Error() == "forbidden" {
		status = http.StatusForbidden
	}
	http.Error(w, `{"code":"charging_request_failed","message":"`+err.Error()+`"}`, status)
}
func (s *Server) CreateChargingStation(w http.ResponseWriter, r *http.Request) {
	var v charging.Station
	if json.NewDecoder(r.Body).Decode(&v) != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	out, err := s.chargingSvc.CreateStation(r.Context(), s.chargingToken(r), v)
	if err != nil {
		s.chargingError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(out)
}
func (s *Server) ListChargingStations(w http.ResponseWriter, r *http.Request) {
	items, total, err := s.store.ListStations(r.Context(), 20, 0)
	if err != nil {
		s.chargingError(w, err)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"items": items, "total": total})
}
func (s *Server) ReportChargingHazard(w http.ResponseWriter, r *http.Request) {
	var v charging.Hazard
	if json.NewDecoder(r.Body).Decode(&v) != nil {
		http.Error(w, "invalid JSON", 400)
		return
	}
	out, err := s.chargingSvc.ReportHazard(r.Context(), s.chargingToken(r), v)
	if err != nil {
		s.chargingError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(out)
}
func (s *Server) AssignChargingHazard(w http.ResponseWriter, r *http.Request) {
	var v struct {
		OperatorID string `json:"operator_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&v)
	err := s.chargingSvc.AssignHazard(r.Context(), s.chargingToken(r), chi.URLParam(r, "id"), v.OperatorID)
	if err != nil {
		s.chargingError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) RectifyChargingHazard(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Evidence string `json:"evidence"`
	}
	_ = json.NewDecoder(r.Body).Decode(&v)
	err := s.chargingSvc.RectifyHazard(r.Context(), s.chargingToken(r), chi.URLParam(r, "id"), v.Evidence)
	if err != nil {
		s.chargingError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) VerifyChargingHazard(w http.ResponseWriter, r *http.Request) {
	var v struct {
		Evidence string `json:"evidence"`
	}
	_ = json.NewDecoder(r.Body).Decode(&v)
	err := s.chargingSvc.VerifyHazard(r.Context(), s.chargingToken(r), chi.URLParam(r, "id"), v.Evidence)
	if err != nil {
		s.chargingError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
