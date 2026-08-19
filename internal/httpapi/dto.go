package httpapi

import (
	"chargeguard/internal/domain"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
)

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeError(w http.ResponseWriter, rerr error) {
	if rerr == nil {
		w.WriteHeader(http.StatusOK)
		return
	}
	status, code := classifyError(rerr)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	writeJSON(w, ErrorResponse{Error: ErrorBody{Code: code, Message: rerr.Error()}})
}

func classifyError(err error) (int, string) {
	var valErr domain.ValidationError
	if errors.As(err, &valErr) {
		return http.StatusBadRequest, "validation_error"
	}
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound, "not_found"
	case errors.Is(err, domain.ErrAlreadyExists):
		return http.StatusConflict, "already_exists"
	case errors.Is(err, domain.ErrConflict):
		return http.StatusConflict, "conflict"
	case errors.Is(err, domain.ErrInvalidTransition):
		return http.StatusConflict, "invalid_transition"
	case errors.Is(err, domain.ErrDuplicateRequest):
		return http.StatusConflict, "duplicate"
	case errors.Is(err, domain.ErrDispositionActive):
		return http.StatusConflict, "disposition_active"
	case errors.Is(err, domain.ErrSignoffIncomplete):
		return http.StatusBadRequest, "signoff_incomplete"
	case errors.Is(err, domain.ErrShardCorrupted):
		return http.StatusInternalServerError, "shard_corrupted"
	case errors.Is(err, domain.ErrSlowConsumer):
		return http.StatusServiceUnavailable, "slow_consumer"
	case errors.Is(err, domain.ErrStaleCheckpoint):
		return http.StatusConflict, "stale_checkpoint"
	case errors.Is(err, domain.ErrPermanentFailure):
		return http.StatusInternalServerError, "permanent_failure"
	default:
		return http.StatusInternalServerError, "internal_error"
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func parsePageSize(s string) int {
	n, _ := strconv.Atoi(s)
	if n <= 0 {
		n = 20
	}
	if n > 200 {
		n = 200
	}
	return n
}

func newPaginated(data any, total, pageSize, offset int) PaginatedResponse {
	return PaginatedResponse{
		Data: data, Total: total, PageSize: pageSize, PageOffset: offset,
		HasNext: offset+pageSize < total,
	}
}

func decodeJSON(r *http.Request, v any) error {
	return json.NewDecoder(r.Body).Decode(v)
}

func respondJSON(w http.ResponseWriter, v any, err error) {
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, v)
}

type PaginatedResponse struct {
	Data       any  `json:"data"`
	Total      int  `json:"total"`
	PageSize   int  `json:"page_size"`
	PageOffset int  `json:"page_offset"`
	HasNext    bool `json:"has_next"`
}

type RegisterHandoverRequest struct {
	FormNo          string        `json:"form_no"`
	Date            string        `json:"date"`
	RouteID         string        `json:"route_id"`
	VehicleNo       string        `json:"vehicle_no"`
	OutboundStation string        `json:"outbound_station"`
	ArrivalStation  string        `json:"arrival_station"`
	Responsible     string        `json:"responsible"`
	MailItems       []MailItemDTO `json:"mail_items"`
}

type MailItemDTO struct {
	MailNo       string `json:"mail_no"`
	SenderName   string `json:"sender_name"`
	SenderAddr   string `json:"sender_addr"`
	ReceiverName string `json:"receiver_name"`
	ReceiverAddr string `json:"receiver_addr"`
}

type SignoffRequest struct {
	Party   string `json:"party"`
	Signer  string `json:"signer"`
	Station string `json:"station"`
}

type SubmitDispositionRequest struct {
	RequestNo     string `json:"request_no"`
	MailID        string `json:"mail_id"`
	Type          string `json:"type"`
	TargetAddress string `json:"target_address"`
	SubmittedBy   string `json:"submitted_by"`
}

type ReviewDispositionRequest struct {
	Reviewer string `json:"reviewer"`
	Decision string `json:"decision"`
	Note     string `json:"note"`
}

type WithdrawDispositionRequest struct {
	WithdrawnBy string `json:"withdrawn_by"`
	Reason      string `json:"reason"`
}

type ExecuteDispositionRequest struct {
	Actor string `json:"actor"`
}

type CreateBatchRequest struct {
	VehicleNo    string `json:"vehicle_no"`
	Date         string `json:"date"`
	RouteID      string `json:"route_id"`
	Dispositions []struct {
		MailID        string `json:"mail_id"`
		Type          string `json:"type"`
		TargetAddress string `json:"target_address"`
	} `json:"dispositions"`
}

type RegisterSubscriberRequest struct {
	SubscriberID   string `json:"subscriber_id"`
	SubscriberType string `json:"subscriber_type"`
	Name           string `json:"name"`
}

type BatchCheckRequest struct {
	MailIDs []string `json:"mail_ids"`
}
