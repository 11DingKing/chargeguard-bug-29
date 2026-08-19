package service

import (
	"chargeguard/internal/circuit"
	"chargeguard/internal/domain"
	"chargeguard/internal/storage"
	"context"
	"fmt"
	"github.com/google/uuid"
)

type DispositionService struct {
	store    storage.Store
	clock    domain.Clock
	eventBus *EventBus
	breaker  *circuit.Breaker
}

func NewDispositionService(store storage.Store, clock domain.Clock, bus *EventBus) *DispositionService {
	return &DispositionService{store: store, clock: clock, eventBus: bus}
}

func (s *DispositionService) WithBreaker(b *circuit.Breaker) *DispositionService {
	s.breaker = b
	return s
}

type SubmitDispositionRequest struct {
	RequestNo     string `json:"request_no"`
	MailID        string `json:"mail_id"`
	Type          string `json:"type"`
	TargetAddress string `json:"target_address,omitempty"`
	SubmittedBy   string `json:"submitted_by"`
}

func (s *DispositionService) Submit(ctx context.Context, req SubmitDispositionRequest) (*domain.DispositionRequest, error) {
	if req.MailID == "" || req.Type == "" || req.SubmittedBy == "" {
		return nil, domain.ValidationError{Field: "mail_id/type/submitted_by", Message: "required fields missing"}
	}
	mail, err := s.store.MailItemRepo().Get(ctx, req.MailID)
	if err != nil {
		return nil, fmt.Errorf("get mail: %w", err)
	}
	if req.RequestNo == "" {
		req.RequestNo = uuid.NewString()
	}
	shardID := mail.ShardID
	now := s.clock.Now()
	disp := &domain.DispositionRequest{
		ID: uuid.NewString(), RequestNo: req.RequestNo, MailID: req.MailID, MailNo: mail.MailNo,
		Type: req.Type, TargetAddress: req.TargetAddress, State: domain.DispositionStatePending,
		SubmittedBy: req.SubmittedBy, SubmittedAt: now, Version: 1, ShardID: shardID, DataVersion: 1,
	}
	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()
	activeCount, err := s.store.DispositionRepo().CountActiveByMailTx(ctx, tx, req.MailID)
	if err != nil {
		return nil, fmt.Errorf("count active: %w", err)
	}
	if activeCount > 0 {
		disp.State = domain.DispositionStateLost
		disp.ConflictReason = fmt.Sprintf("another active disposition already exists for mail %s", mail.MailNo)
		disp.LostAt = now
		if err := s.store.DispositionRepo().SaveTx(ctx, tx, disp); err != nil {
			return nil, fmt.Errorf("save lost disposition: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit: %w", err)
		}
		sharedRecordLedger(ctx, s.store, s.clock, shardID, "", disp.MailNo, req.SubmittedBy,
			domain.LedgerEntryTypeDisposition, domain.DispositionStatePending, disp.State)
		sharedAppendAudit(ctx, s.store, s.clock, req.SubmittedBy, domain.AdjudicationActionLost, domain.EntityTypeDisposition, disp.ID, shardID, "", disp.State, disp.ConflictReason)
		sharedPublishEvent(ctx, s.eventBus, s.clock, domain.EventDispositionLost, disp.ID, shardID, disp)
		return disp, nil
	}
	if err := s.store.DispositionRepo().SaveTx(ctx, tx, disp); err != nil {
		return nil, fmt.Errorf("save disposition: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	sharedRecordLedger(ctx, s.store, s.clock, shardID, "", disp.MailNo, req.SubmittedBy,
		domain.LedgerEntryTypeDisposition, "", disp.State)
	sharedAppendAudit(ctx, s.store, s.clock, req.SubmittedBy, domain.AdjudicationActionSubmit, domain.EntityTypeDisposition, disp.ID, shardID, "", disp.State, "submitted")
	sharedPublishEvent(ctx, s.eventBus, s.clock, domain.EventDispositionSubmitted, disp.ID, shardID, disp)
	return disp, nil
}

type ReviewRequest struct {
	DispositionID string `json:"disposition_id"`
	Reviewer      string `json:"reviewer"`
	Decision      string `json:"decision"`
	Note          string `json:"note"`
}

func (s *DispositionService) Review(ctx context.Context, req ReviewRequest) (*domain.DispositionRequest, error) {
	disp, err := s.store.DispositionRepo().Get(ctx, req.DispositionID)
	if err != nil {
		return nil, fmt.Errorf("get disposition: %w", err)
	}
	var targetState string
	switch req.Decision {
	case "approve":
		targetState = domain.DispositionStateIssued
	case "reject":
		targetState = domain.DispositionStateRejected
	default:
		return nil, domain.ValidationError{Field: "decision", Message: "must be approve or reject"}
	}
	if err := domain.ValidateDispositionTransition(disp.State, targetState); err != nil {
		return nil, fmt.Errorf("validate transition: %w", err)
	}
	prevState := disp.State
	disp.State = targetState
	disp.ReviewedBy = req.Reviewer
	disp.ReviewedAt = s.clock.Now()
	disp.ReviewNote = req.Note
	disp.Version++
	if targetState == domain.DispositionStateIssued {
		disp.IssuedBy = req.Reviewer
		disp.IssuedAt = s.clock.Now()
	}
	if err := s.store.DispositionRepo().Save(ctx, disp); err != nil {
		return nil, fmt.Errorf("save disposition: %w", err)
	}
	sharedRecordLedger(ctx, s.store, s.clock, disp.ShardID, "", disp.MailNo, req.Reviewer,
		domain.LedgerEntryTypeDisposition, prevState, disp.State)
	sharedAppendAudit(ctx, s.store, s.clock, req.Reviewer, domain.AdjudicationActionReview, domain.EntityTypeDisposition, disp.ID, disp.ShardID, prevState, disp.State, req.Note)
	sharedPublishEvent(ctx, s.eventBus, s.clock, domain.EventDispositionReviewed, disp.ID, disp.ShardID, disp)
	return disp, nil
}

type WithdrawRequest struct {
	DispositionID string `json:"disposition_id"`
	WithdrawnBy   string `json:"withdrawn_by"`
	Reason        string `json:"reason"`
}

func (s *DispositionService) Withdraw(ctx context.Context, req WithdrawRequest) (*domain.DispositionRequest, error) {
	disp, err := s.store.DispositionRepo().Get(ctx, req.DispositionID)
	if err != nil {
		return nil, fmt.Errorf("get disposition: %w", err)
	}
	if err := domain.ValidateDispositionTransition(disp.State, domain.DispositionStateWithdrawn); err != nil {
		return nil, fmt.Errorf("validate transition: %w", err)
	}
	prevState := disp.State
	disp.State = domain.DispositionStateWithdrawn
	disp.WithdrawnBy = req.WithdrawnBy
	disp.WithdrawnAt = s.clock.Now()
	disp.WithdrawnReason = req.Reason
	disp.Version++
	if err := s.store.DispositionRepo().Save(ctx, disp); err != nil {
		return nil, fmt.Errorf("save disposition: %w", err)
	}
	sharedRecordLedger(ctx, s.store, s.clock, disp.ShardID, "", disp.MailNo, req.WithdrawnBy,
		domain.LedgerEntryTypeWithdrawal, prevState, disp.State)
	sharedAppendAudit(ctx, s.store, s.clock, req.WithdrawnBy, domain.AdjudicationActionWithdraw, domain.EntityTypeDisposition, disp.ID, disp.ShardID, prevState, disp.State, req.Reason)
	sharedPublishEvent(ctx, s.eventBus, s.clock, domain.EventDispositionWithdrawn, disp.ID, disp.ShardID, disp)
	return disp, nil
}

func (s *DispositionService) Execute(ctx context.Context, dispID, actor string) (*domain.DispositionRequest, error) {
	if s.breaker != nil {
		var result *domain.DispositionRequest
		err := s.breaker.Execute(func() error {
			r, e := s.executeInternal(ctx, dispID, actor)
			result = r
			return e
		})
		return result, err
	}
	return s.executeInternal(ctx, dispID, actor)
}

func (s *DispositionService) executeInternal(ctx context.Context, dispID, actor string) (*domain.DispositionRequest, error) {
	disp, err := s.store.DispositionRepo().Get(ctx, dispID)
	if err != nil {
		return nil, fmt.Errorf("get disposition: %w", err)
	}
	if err := domain.ValidateDispositionTransition(disp.State, domain.DispositionStateExecuting); err != nil {
		return nil, fmt.Errorf("validate transition: %w", err)
	}
	prevState := disp.State
	disp.State = domain.DispositionStateExecuting
	disp.ExecutedAt = s.clock.Now()
	disp.Version++
	if err := s.store.DispositionRepo().Save(ctx, disp); err != nil {
		return nil, fmt.Errorf("save disposition: %w", err)
	}
	mail, err := s.store.MailItemRepo().Get(ctx, disp.MailID)
	if err != nil {
		return nil, fmt.Errorf("get mail: %w", err)
	}
	mailPrevState := mail.State
	switch disp.Type {
	case domain.DispositionTypeIntercept:
		mail.State = domain.MailStateIntercepted
	case domain.DispositionTypeRedirect:
		mail.State = domain.MailStateRedirected
		if disp.TargetAddress != "" {
			mail.ReceiverAddr = disp.TargetAddress
		}
	case domain.DispositionTypeReturn:
		mail.State = domain.MailStateReturnedToSender
	}
	mail.DispositionID = disp.ID
	mail.Version++
	if err := s.store.MailItemRepo().Save(ctx, mail); err != nil {
		return nil, fmt.Errorf("save mail: %w", err)
	}
	disp.State = domain.DispositionStateCompleted
	disp.CompletedAt = s.clock.Now()
	disp.Version++
	if err := s.store.DispositionRepo().Save(ctx, disp); err != nil {
		return nil, fmt.Errorf("complete disposition: %w", err)
	}
	sharedRecordLedger(ctx, s.store, s.clock, disp.ShardID, "", disp.MailNo, actor,
		domain.LedgerEntryTypeDisposition, prevState, disp.State)
	sharedAppendAudit(ctx, s.store, s.clock, actor, domain.AdjudicationActionExecute, domain.EntityTypeDisposition, disp.ID, disp.ShardID, prevState, disp.State, "")
	sharedAppendAudit(ctx, s.store, s.clock, actor, domain.AdjudicationActionComplete, domain.EntityTypeDisposition, disp.ID, disp.ShardID, mailPrevState, mail.State, "")
	sharedPublishEvent(ctx, s.eventBus, s.clock, domain.EventDispositionCompleted, disp.ID, disp.ShardID, disp)
	return disp, nil
}

func (s *DispositionService) Get(ctx context.Context, id string) (*domain.DispositionRequest, error) {
	return s.store.DispositionRepo().Get(ctx, id)
}

func (s *DispositionService) List(ctx context.Context, filter storage.DispositionFilter) ([]*domain.DispositionRequest, int, error) {
	return s.store.DispositionRepo().List(ctx, filter)
}

func (s *DispositionService) GetActiveByMail(ctx context.Context, mailID string) ([]*domain.DispositionRequest, error) {
	return s.store.DispositionRepo().GetActiveByMail(ctx, mailID)
}
