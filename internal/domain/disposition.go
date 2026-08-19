package domain

import (
	"time"
)

const (
	DispositionTypeRedirect  = "redirect"
	DispositionTypeIntercept = "intercept"
	DispositionTypeReturn    = "return"
)

const (
	DispositionStatePending     = "pending"
	DispositionStateUnderReview = "under_review"
	DispositionStateIssued      = "issued"
	DispositionStateExecuting   = "executing"
	DispositionStateCompleted   = "completed"
	DispositionStateRejected    = "rejected"
	DispositionStateWithdrawn   = "withdrawn"
	DispositionStateLost        = "lost"
)

var dispositionStateMachine = NewStateMachine("disposition", DispositionStatePending,
	StateTransition{DispositionStatePending, DispositionStateUnderReview},
	StateTransition{DispositionStatePending, DispositionStateIssued},
	StateTransition{DispositionStatePending, DispositionStateRejected},
	StateTransition{DispositionStateUnderReview, DispositionStateIssued},
	StateTransition{DispositionStateUnderReview, DispositionStateRejected},
	StateTransition{DispositionStateIssued, DispositionStateExecuting},
	StateTransition{DispositionStateExecuting, DispositionStateCompleted},
	StateTransition{DispositionStateIssued, DispositionStateWithdrawn},
	StateTransition{DispositionStateExecuting, DispositionStateWithdrawn},
	StateTransition{DispositionStatePending, DispositionStateWithdrawn},
	StateTransition{DispositionStatePending, DispositionStateLost},
	StateTransition{DispositionStateUnderReview, DispositionStateLost},
	StateTransition{DispositionStateWithdrawn, DispositionStatePending},
)

type DispositionRequest struct {
	ID              string    `json:"id"`
	RequestNo       string    `json:"request_no"`
	MailID          string    `json:"mail_id"`
	MailNo          string    `json:"mail_no"`
	Type            string    `json:"type"`
	TargetAddress   string    `json:"target_address,omitempty"`
	State           string    `json:"state"`
	SubmittedBy     string    `json:"submitted_by"`
	SubmittedAt     time.Time `json:"submitted_at"`
	ReviewedBy      string    `json:"reviewed_by,omitempty"`
	ReviewedAt      time.Time `json:"reviewed_at,omitempty"`
	ReviewNote      string    `json:"review_note,omitempty"`
	IssuedBy        string    `json:"issued_by,omitempty"`
	IssuedAt        time.Time `json:"issued_at,omitempty"`
	ExecutedAt      time.Time `json:"executed_at,omitempty"`
	CompletedAt     time.Time `json:"completed_at,omitempty"`
	WithdrawnBy     string    `json:"withdrawn_by,omitempty"`
	WithdrawnAt     time.Time `json:"withdrawn_at,omitempty"`
	WithdrawnReason string    `json:"withdrawn_reason,omitempty"`
	ConflictReason  string    `json:"conflict_reason,omitempty"`
	LostAt          time.Time `json:"lost_at,omitempty"`
	Version         int       `json:"version"`
	ShardID         string    `json:"shard_id"`
	DataVersion     int       `json:"data_version"`
}

const (
	AdjudicationActionSubmit   = "submit"
	AdjudicationActionReview   = "review"
	AdjudicationActionExecute  = "execute"
	AdjudicationActionComplete = "complete"
	AdjudicationActionWithdraw = "withdraw"
	AdjudicationActionLost     = "lost"
)

func ValidateDispositionTransition(current, target string) error {
	return dispositionStateMachine.Validate(current, target)
}

func IsDispositionActive(state string) bool {
	switch state {
	case DispositionStatePending, DispositionStateUnderReview,
		DispositionStateIssued, DispositionStateExecuting:
		return true
	default:
		return false
	}
}
