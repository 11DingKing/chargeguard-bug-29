package httpapi

import (
	"chargeguard/internal/service"
	"chargeguard/internal/storage"
	"github.com/go-chi/chi/v5"
	"net/http"
	"strconv"
	"time"
)

func (s *Server) ListMails(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	pageSize := parsePageSize(q.Get("page_size"))
	pageOffset, _ := strconv.Atoi(q.Get("page_offset"))
	filter := storage.MailFilter{
		State: q.Get("state"), RouteID: q.Get("route_id"), VehicleNo: q.Get("vehicle_no"),
		PageSize: pageSize, PageOffset: pageOffset,
	}
	if st := q.Get("start_time"); st != "" {
		if t, err := time.Parse(time.RFC3339, st); err == nil {
			filter.StartTime = t
		}
	}
	if et := q.Get("end_time"); et != "" {
		if t, err := time.Parse(time.RFC3339, et); err == nil {
			filter.EndTime = t
		}
	}
	mails, total, err := s.store.MailItemRepo().List(r.Context(), filter)
	respondJSON(w, newPaginated(mails, total, pageSize, pageOffset), err)
}

func (s *Server) GetMail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	mail, err := s.store.MailItemRepo().Get(r.Context(), id)
	respondJSON(w, mail, err)
}

func (s *Server) GetMailDispositions(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	dispS, err := s.dispSvc.GetActiveByMail(r.Context(), id)
	respondJSON(w, dispS, err)
}

func (s *Server) CreateBatch(w http.ResponseWriter, r *http.Request) {
	var req CreateBatchRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	batch, err := s.batchSvc.CreateBatch(r.Context(), service.CreateBatchRequest{
		VehicleNo: req.VehicleNo, Date: req.Date, RouteID: req.RouteID,
	})
	respondJSON(w, batch, err)
}

func (s *Server) ProcessBatch(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	batch, err := s.batchSvc.ProcessBatch(r.Context(), id)
	respondJSON(w, batch, err)
}

func (s *Server) GetBatch(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	batch, err := s.batchSvc.GetBatch(r.Context(), id)
	respondJSON(w, batch, err)
}

func (s *Server) ListBatchItems(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	items, err := s.batchSvc.ListItems(r.Context(), id)
	respondJSON(w, items, err)
}
