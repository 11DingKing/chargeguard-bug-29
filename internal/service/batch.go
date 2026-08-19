package service

import (
	"chargeguard/internal/domain"
	"chargeguard/internal/storage"
	"context"
	"fmt"
	"github.com/google/uuid"
)

type BatchService struct {
	store    storage.Store
	clock    domain.Clock
	eventBus *EventBus
}

func NewBatchService(store storage.Store, clock domain.Clock, bus *EventBus) *BatchService {
	return &BatchService{store: store, clock: clock, eventBus: bus}
}

type CreateBatchRequest struct {
	VehicleNo    string   `json:"vehicle_no"`
	Date         string   `json:"date"`
	RouteID      string   `json:"route_id"`
	MailIDs      []string `json:"mail_ids"`
	Dispositions []struct {
		MailID        string `json:"mail_id"`
		Type          string `json:"type"`
		TargetAddress string `json:"target_address"`
	} `json:"dispositions"`
}

func (s *BatchService) CreateBatch(ctx context.Context, req CreateBatchRequest) (*domain.BatchRecord, error) {
	if req.VehicleNo == "" || req.Date == "" || req.RouteID == "" {
		return nil, domain.ValidationError{Field: "vehicle_no/date/route_id", Message: "required"}
	}
	shardID := domain.ShardIDFor(req.Date, req.RouteID)
	now := s.clock.Now()
	itemCount := len(req.Dispositions)
	if itemCount == 0 {
		itemCount = len(req.MailIDs)
	}
	batch := &domain.BatchRecord{
		ID: uuid.NewString(), VehicleNo: req.VehicleNo, Date: req.Date, RouteID: req.RouteID,
		State: domain.BatchStatePending, TotalCount: itemCount,
		CreatedAt: now, UpdatedAt: now, Version: 1, ShardID: shardID, DataVersion: 1,
	}
	if err := s.store.BatchRepo().Save(ctx, batch); err != nil {
		return nil, fmt.Errorf("save batch: %w", err)
	}
	mailIDs := req.MailIDs
	if len(req.Dispositions) > 0 {
		mailIDs = make([]string, 0, len(req.Dispositions))
		for _, d := range req.Dispositions {
			mailIDs = append(mailIDs, d.MailID)
		}
	}
	for _, mailID := range mailIDs {
		mail, err := s.store.MailItemRepo().Get(ctx, mailID)
		if err != nil {
			return nil, fmt.Errorf("get mail %s: %w", mailID, err)
		}
		item := &domain.BatchItem{
			ID: uuid.NewString(), BatchID: batch.ID, MailID: mailID, MailNo: mail.MailNo,
			State: domain.BatchItemStatePending, CreatedAt: now, UpdatedAt: now,
		}
		if err := s.store.BatchRepo().SaveItem(ctx, item); err != nil {
			return nil, fmt.Errorf("save batch item: %w", err)
		}
	}
	return batch, nil
}

func (s *BatchService) ProcessBatch(ctx context.Context, batchID string) (*domain.BatchRecord, error) {
	batch, err := s.store.BatchRepo().Get(ctx, batchID)
	if err != nil {
		return nil, fmt.Errorf("get batch: %w", err)
	}
	if err := domain.ValidateBatchTransition(batch.State, domain.BatchStateProcessing); err != nil {
		return nil, fmt.Errorf("validate batch transition: %w", err)
	}
	batch.State = domain.BatchStateProcessing
	batch.UpdatedAt = s.clock.Now()
	batch.Version++
	if err := s.store.BatchRepo().Save(ctx, batch); err != nil {
		return nil, fmt.Errorf("save batch processing: %w", err)
	}
	items, err := s.store.BatchRepo().ListItems(ctx, batchID)
	if err != nil {
		return nil, fmt.Errorf("list items: %w", err)
	}
	processedThisAttempt := make(map[string]bool)
	allSucceeded := true
	for _, item := range items {
		if item.State == domain.BatchItemStateSucceeded {
			continue
		}
		if err := s.processItem(ctx, item); err != nil {
			item.State = domain.BatchItemStateFailed
			item.Error = err.Error()
			item.UpdatedAt = s.clock.Now()
			s.store.BatchRepo().SaveItem(ctx, item)
			allSucceeded = false
			continue
		}
		item.State = domain.BatchItemStateSucceeded
		item.UpdatedAt = s.clock.Now()
		s.store.BatchRepo().SaveItem(ctx, item)
		processedThisAttempt[item.ID] = true
	}
	if allSucceeded {
		batch.State = domain.BatchStateSucceeded
		batch.SucceededCount = len(items)
		batch.UpdatedAt = s.clock.Now()
		batch.Version++
		s.store.BatchRepo().Save(ctx, batch)
		batch.State = domain.BatchStateCompleted
		batch.Version++
		s.store.BatchRepo().Save(ctx, batch)
		sharedPublishEvent(ctx, s.eventBus, s.clock, domain.EventBatchProcessed, batch.ID, batch.ShardID, batch)
		return batch, nil
	}
	for _, item := range items {
		if processedThisAttempt[item.ID] {
			s.compensateItem(ctx, item)
			dbItem, _ := s.store.BatchRepo().GetItem(ctx, item.ID)
			dbItem.State = domain.BatchItemStateRolledBack
			dbItem.UpdatedAt = s.clock.Now()
			s.store.BatchRepo().SaveItem(ctx, dbItem)
		}
	}
	succeeded := 0
	failed := 0
	for _, item := range items {
		switch item.State {
		case domain.BatchItemStateSucceeded:
			succeeded++
		default:
			failed++
		}
	}
	batch.State = domain.BatchStateRolledBack
	batch.SucceededCount = succeeded
	batch.FailedCount = failed
	batch.UpdatedAt = s.clock.Now()
	batch.Version++
	s.store.BatchRepo().Save(ctx, batch)
	sharedPublishEvent(ctx, s.eventBus, s.clock, domain.EventBatchRolledBack, batch.ID, batch.ShardID, batch)
	return batch, nil
}

func (s *BatchService) processItem(ctx context.Context, item *domain.BatchItem) error {
	mail, err := s.store.MailItemRepo().Get(ctx, item.MailID)
	if err != nil {
		return fmt.Errorf("get mail: %w", err)
	}
	if mail.State != domain.MailStateInTransit && mail.State != domain.MailStateArrived {
		return fmt.Errorf("mail %s in state %s, cannot process", mail.MailNo, mail.State)
	}
	mail.State = domain.MailStateArrived
	mail.Version++
	return s.store.MailItemRepo().Save(ctx, mail)
}

func (s *BatchService) compensateItem(ctx context.Context, item *domain.BatchItem) {
	mail, err := s.store.MailItemRepo().Get(ctx, item.MailID)
	if err != nil {
		return
	}
	if mail.State == domain.MailStateArrived {
		mail.State = domain.MailStateInTransit
		mail.Version++
		s.store.MailItemRepo().Save(ctx, mail)
	}
	sharedAppendAudit(ctx, s.store, s.clock, "system", "compensate", domain.EntityTypeBatch, item.BatchID, mail.ShardID,
		item.State, domain.BatchItemStateRolledBack, "batch compensation")
}

func (s *BatchService) CompensateItem(ctx context.Context, item *domain.BatchItem) {
	s.compensateItem(ctx, item)
}

func (s *BatchService) GetBatch(ctx context.Context, id string) (*domain.BatchRecord, error) {
	return s.store.BatchRepo().Get(ctx, id)
}

func (s *BatchService) ListItems(ctx context.Context, batchID string) ([]*domain.BatchItem, error) {
	return s.store.BatchRepo().ListItems(ctx, batchID)
}

func (s *BatchService) ListPending(ctx context.Context) ([]*domain.BatchRecord, error) {
	return s.store.BatchRepo().ListPending(ctx)
}
