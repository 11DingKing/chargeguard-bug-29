package httpapi

import (
	"chargeguard/internal/domain"
	"chargeguard/internal/service"
	"chargeguard/internal/storage"
	"github.com/go-chi/chi/v5"
	"net/http"
	"strconv"
)

func (s *Server) RegisterHandover(w http.ResponseWriter, r *http.Request) {
	var req RegisterHandoverRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	mailItems := make([]service.RegisterMailItem, len(req.MailItems))
	for i, mi := range req.MailItems {
		mailItems[i] = service.RegisterMailItem{
			MailNo: mi.MailNo, SenderName: mi.SenderName, SenderAddr: mi.SenderAddr,
			ReceiverName: mi.ReceiverName, ReceiverAddr: mi.ReceiverAddr,
		}
	}
	form, err := s.handSvc.Register(r.Context(), service.RegisterHandoverRequest{
		FormNo: req.FormNo, Date: req.Date, RouteID: req.RouteID, VehicleNo: req.VehicleNo,
		OutboundStation: req.OutboundStation, ArrivalStation: req.ArrivalStation,
		Responsible: req.Responsible, MailItems: mailItems,
	})
	respondJSON(w, form, err)
}

func (s *Server) GetHandover(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	form, err := s.handSvc.Get(r.Context(), id)
	respondJSON(w, form, err)
}

func (s *Server) ListHandovers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	pageSize := parsePageSize(q.Get("page_size"))
	pageOffset, _ := strconv.Atoi(q.Get("page_offset"))
	filter := storage.HandoverFilter{
		State: q.Get("state"), RouteID: q.Get("route_id"), Date: q.Get("date"),
		PageSize: pageSize, PageOffset: pageOffset,
	}
	forms, total, err := s.handSvc.List(r.Context(), filter)
	respondJSON(w, newPaginated(forms, total, pageSize, pageOffset), err)
}

func (s *Server) ModifyHandover(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req RegisterHandoverRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	form, err := s.handSvc.Get(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	if form.State != domain.HandoverStateDraft {
		writeError(w, domain.ErrInvalidTransition)
		return
	}
	form.VehicleNo = req.VehicleNo
	form.OutboundStation = req.OutboundStation
	form.ArrivalStation = req.ArrivalStation
	form.Responsible = req.Responsible
	form.UpdatedAt = form.RegisteredAt
	form.Version++
	if err := s.handSvc.ModifySave(r.Context(), form); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, form)
}

func (s *Server) HandoverSignoff(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req SignoffRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	form, err := s.handSvc.SignoffWithLock(r.Context(), domain.HandoverSignoffRequest{
		FormID: id, Party: req.Party, Signer: req.Signer, Station: req.Station,
	})
	respondJSON(w, form, err)
}

func (s *Server) SubmitDisposition(w http.ResponseWriter, r *http.Request) {
	var req SubmitDispositionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	disp, err := s.dispSvc.Submit(r.Context(), service.SubmitDispositionRequest{
		RequestNo: req.RequestNo, MailID: req.MailID, Type: req.Type,
		TargetAddress: req.TargetAddress, SubmittedBy: req.SubmittedBy,
	})
	respondJSON(w, disp, err)
}

func (s *Server) GetDisposition(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	disp, err := s.dispSvc.Get(r.Context(), id)
	respondJSON(w, disp, err)
}

func (s *Server) ListDispositions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	pageSize := parsePageSize(q.Get("page_size"))
	pageOffset, _ := strconv.Atoi(q.Get("page_offset"))
	filter := storage.DispositionFilter{
		State: q.Get("state"), MailID: q.Get("mail_id"),
		SubmittedBy: q.Get("submitted_by"),
		PageSize:    pageSize, PageOffset: pageOffset,
	}
	dispS, total, err := s.dispSvc.List(r.Context(), filter)
	respondJSON(w, newPaginated(dispS, total, pageSize, pageOffset), err)
}

func (s *Server) ReviewDisposition(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req ReviewDispositionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	disp, err := s.dispSvc.Review(r.Context(), service.ReviewRequest{
		DispositionID: id, Reviewer: req.Reviewer, Decision: req.Decision, Note: req.Note,
	})
	respondJSON(w, disp, err)
}

func (s *Server) WithdrawDisposition(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req WithdrawDispositionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	disp, err := s.dispSvc.Withdraw(r.Context(), service.WithdrawRequest{
		DispositionID: id, WithdrawnBy: req.WithdrawnBy, Reason: req.Reason,
	})
	respondJSON(w, disp, err)
}

func (s *Server) ExecuteDisposition(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req ExecuteDispositionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	disp, err := s.dispSvc.Execute(r.Context(), id, req.Actor)
	respondJSON(w, disp, err)
}
