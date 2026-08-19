package domain

import (
	"time"
)

const (
	MailStateRegistered       = "registered"
	MailStateLoaded           = "loaded"
	MailStateInTransit        = "in_transit"
	MailStateArrived          = "arrived"
	MailStateDualSigned       = "dual_signed"
	MailStateCompleted        = "completed"
	MailStateIntercepted      = "intercepted"
	MailStateRedirected       = "redirected"
	MailStateReturnedToSender = "returned_to_sender"
)

var mailStateMachine = NewStateMachine("mail", MailStateRegistered,
	StateTransition{MailStateRegistered, MailStateLoaded},
	StateTransition{MailStateLoaded, MailStateInTransit},
	StateTransition{MailStateInTransit, MailStateArrived},
	StateTransition{MailStateInTransit, MailStateIntercepted},
	StateTransition{MailStateInTransit, MailStateRedirected},
	StateTransition{MailStateArrived, MailStateDualSigned},
	StateTransition{MailStateRedirected, MailStateArrived},
	StateTransition{MailStateDualSigned, MailStateCompleted},
	StateTransition{MailStateIntercepted, MailStateReturnedToSender},
	StateTransition{MailStateReturnedToSender, MailStateCompleted},
)

type MailItem struct {
	ID            string    `json:"id"`
	MailNo        string    `json:"mail_no"`
	RouteID       string    `json:"route_id"`
	VehicleNo     string    `json:"vehicle_no"`
	State         string    `json:"state"`
	HandoverID    string    `json:"handover_id,omitempty"`
	DispositionID string    `json:"disposition_id,omitempty"`
	OriginStation string    `json:"origin_station"`
	DestStation   string    `json:"dest_station"`
	SenderName    string    `json:"sender_name"`
	SenderAddr    string    `json:"sender_addr"`
	ReceiverName  string    `json:"receiver_name"`
	ReceiverAddr  string    `json:"receiver_addr"`
	Responsible   string    `json:"responsible"`
	RegisteredAt  time.Time `json:"registered_at"`
	LoadedAt      time.Time `json:"loaded_at,omitempty"`
	InTransitAt   time.Time `json:"in_transit_at,omitempty"`
	ArrivedAt     time.Time `json:"arrived_at,omitempty"`
	SignedAt      time.Time `json:"signed_at,omitempty"`
	CompletedAt   time.Time `json:"completed_at,omitempty"`
	Version       int       `json:"version"`
	ShardID       string    `json:"shard_id"`
	DataVersion   int       `json:"data_version"`
}

func ValidateMailTransition(current, target string) error {
	return mailStateMachine.Validate(current, target)
}

const (
	HandoverStateDraft          = "draft"
	HandoverStateOutboundSigned = "outbound_signed"
	HandoverStateArrivalSigned  = "arrival_signed"
	HandoverStateDualSigned     = "dual_signed"
	HandoverStateVoided         = "voided"
)

var handoverStateMachine = NewStateMachine("handover", HandoverStateDraft,
	StateTransition{HandoverStateDraft, HandoverStateOutboundSigned},
	StateTransition{HandoverStateDraft, HandoverStateArrivalSigned},
	StateTransition{HandoverStateOutboundSigned, HandoverStateArrivalSigned},
	StateTransition{HandoverStateArrivalSigned, HandoverStateOutboundSigned},
	StateTransition{HandoverStateOutboundSigned, HandoverStateDualSigned},
	StateTransition{HandoverStateArrivalSigned, HandoverStateDualSigned},
	StateTransition{HandoverStateOutboundSigned, HandoverStateVoided},
	StateTransition{HandoverStateArrivalSigned, HandoverStateVoided},
	StateTransition{HandoverStateDraft, HandoverStateVoided},
)

type HandoverForm struct {
	ID               string    `json:"id"`
	FormNo           string    `json:"form_no"`
	Date             string    `json:"date"`
	RouteID          string    `json:"route_id"`
	VehicleNo        string    `json:"vehicle_no"`
	State            string    `json:"state"`
	OutboundStation  string    `json:"outbound_station"`
	OutboundSigner   string    `json:"outbound_signer"`
	OutboundSignedAt time.Time `json:"outbound_signed_at,omitempty"`
	ArrivalStation   string    `json:"arrival_station"`
	ArrivalSigner    string    `json:"arrival_signer"`
	ArrivalSignedAt  time.Time `json:"arrival_signed_at,omitempty"`
	MailItemCount    int       `json:"mail_item_count"`
	Responsible      string    `json:"responsible"`
	RegisteredAt     time.Time `json:"registered_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	Version          int       `json:"version"`
	ShardID          string    `json:"shard_id"`
	DataVersion      int       `json:"data_version"`
}

type HandoverSignoffRequest struct {
	FormID  string `json:"form_id"`
	Party   string `json:"party"`
	Signer  string `json:"signer"`
	Station string `json:"station"`
}

const (
	SignoffPartyOutbound = "outbound"
	SignoffPartyArrival  = "arrival"
)

func ValidateHandoverTransition(current, target string) error {
	return handoverStateMachine.Validate(current, target)
}

func IsHandoverComplete(state string) bool {
	return state == HandoverStateDualSigned
}

func HandoverNextStateAfterSignoff(current, party string) string {
	switch {
	case current == HandoverStateDraft && party == SignoffPartyOutbound:
		return HandoverStateOutboundSigned
	case current == HandoverStateDraft && party == SignoffPartyArrival:
		return HandoverStateArrivalSigned
	case current == HandoverStateOutboundSigned && party == SignoffPartyArrival:
		return HandoverStateDualSigned
	case current == HandoverStateArrivalSigned && party == SignoffPartyOutbound:
		return HandoverStateDualSigned
	default:
		return current
	}
}

const (
	BatchStatePending    = "pending"
	BatchStateProcessing = "processing"
	BatchStateSucceeded  = "succeeded"
	BatchStateRolledBack = "rolled_back"
	BatchStateCompleted  = "completed"
)

var batchStateMachine = NewStateMachine("batch", BatchStatePending,
	StateTransition{BatchStatePending, BatchStateProcessing},
	StateTransition{BatchStateProcessing, BatchStateSucceeded},
	StateTransition{BatchStateProcessing, BatchStateRolledBack},
	StateTransition{BatchStateRolledBack, BatchStateProcessing},
	StateTransition{BatchStateSucceeded, BatchStateCompleted},
)

const (
	BatchItemStatePending    = "pending"
	BatchItemStateSucceeded  = "succeeded"
	BatchItemStateFailed     = "failed"
	BatchItemStateRolledBack = "rolled_back"
)

type BatchRecord struct {
	ID             string    `json:"id"`
	VehicleNo      string    `json:"vehicle_no"`
	Date           string    `json:"date"`
	RouteID        string    `json:"route_id"`
	State          string    `json:"state"`
	TotalCount     int       `json:"total_count"`
	SucceededCount int       `json:"succeeded_count"`
	FailedCount    int       `json:"failed_count"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	Version        int       `json:"version"`
	ShardID        string    `json:"shard_id"`
	DataVersion    int       `json:"data_version"`
}

type BatchItem struct {
	ID        string    `json:"id"`
	BatchID   string    `json:"batch_id"`
	MailID    string    `json:"mail_id"`
	MailNo    string    `json:"mail_no"`
	State     string    `json:"state"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func ValidateBatchTransition(current, target string) error {
	return batchStateMachine.Validate(current, target)
}
