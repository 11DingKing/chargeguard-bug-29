package service

import (
	"chargeguard/internal/domain"
	"chargeguard/internal/storage"
	"context"
	"fmt"
	"github.com/google/uuid"
	"sync"
)

type HandoverService struct {
	store    storage.Store
	clock    domain.Clock
	eventBus *EventBus
}

func NewHandoverService(store storage.Store, clock domain.Clock, bus *EventBus) *HandoverService {
	return &HandoverService{store: store, clock: clock, eventBus: bus}
}

type RegisterHandoverRequest struct {
	FormNo          string             `json:"form_no"`
	Date            string             `json:"date"`
	RouteID         string             `json:"route_id"`
	VehicleNo       string             `json:"vehicle_no"`
	OutboundStation string             `json:"outbound_station"`
	ArrivalStation  string             `json:"arrival_station"`
	Responsible     string             `json:"responsible"`
	MailItems       []RegisterMailItem `json:"mail_items"`
}

type RegisterMailItem struct {
	MailNo       string `json:"mail_no"`
	SenderName   string `json:"sender_name"`
	SenderAddr   string `json:"sender_addr"`
	ReceiverName string `json:"receiver_name"`
	ReceiverAddr string `json:"receiver_addr"`
}

func (s *HandoverService) Register(ctx context.Context, req RegisterHandoverRequest) (*domain.HandoverForm, error) {
	if req.FormNo == "" || req.Date == "" || req.RouteID == "" {
		return nil, domain.ValidationError{Field: "form_no/date/route_id", Message: "required fields missing"}
	}
	if _, err := s.store.HandoverRepo().GetByFormNo(ctx, req.FormNo); err == nil {
		return nil, fmt.Errorf("%w: form_no %s", domain.ErrAlreadyExists, req.FormNo)
	}
	shardID := domain.ShardIDFor(req.Date, req.RouteID)
	now := s.clock.Now()
	form := &domain.HandoverForm{
		ID: uuid.NewString(), FormNo: req.FormNo, Date: req.Date, RouteID: req.RouteID,
		VehicleNo: req.VehicleNo, State: domain.HandoverStateDraft,
		OutboundStation: req.OutboundStation, ArrivalStation: req.ArrivalStation,
		MailItemCount: len(req.MailItems), Responsible: req.Responsible,
		RegisteredAt: now, UpdatedAt: now, Version: 1, ShardID: shardID, DataVersion: 1,
	}
	if err := s.store.HandoverRepo().Save(ctx, form); err != nil {
		return nil, fmt.Errorf("save handover: %w", err)
	}
	for _, mi := range req.MailItems {
		mail := &domain.MailItem{
			ID: uuid.NewString(), MailNo: mi.MailNo, RouteID: req.RouteID, VehicleNo: req.VehicleNo,
			State: domain.MailStateRegistered, HandoverID: form.ID,
			OriginStation: req.OutboundStation, DestStation: req.ArrivalStation,
			SenderName: mi.SenderName, SenderAddr: mi.SenderAddr,
			ReceiverName: mi.ReceiverName, ReceiverAddr: mi.ReceiverAddr,
			Responsible: req.Responsible, RegisteredAt: now, Version: 1,
			ShardID: shardID, DataVersion: 1,
		}
		if err := s.store.MailItemRepo().Save(ctx, mail); err != nil {
			return nil, fmt.Errorf("save mail %s: %w", mi.MailNo, err)
		}
		sharedRecordLedger(ctx, s.store, s.clock, shardID, req.FormNo, mi.MailNo,
			req.Responsible, domain.LedgerEntryTypeRegistration, "", domain.MailStateRegistered)
	}
	sharedAppendAudit(ctx, s.store, s.clock, req.Responsible, "register", domain.EntityTypeHandover, form.ID, shardID, "", form.State,
		fmt.Sprintf("form %s with %d mails", req.FormNo, len(req.MailItems)))
	sharedPublishEvent(ctx, s.eventBus, s.clock, domain.EventHandoverRegistered, form.ID, shardID, form)
	return form, nil
}

func (s *HandoverService) Signoff(ctx context.Context, req domain.HandoverSignoffRequest) (*domain.HandoverForm, error) {
	form, err := s.store.HandoverRepo().Get(ctx, req.FormID)
	if err != nil {
		return nil, fmt.Errorf("get handover: %w", err)
	}
	if form.State == domain.HandoverStateDualSigned {
		return nil, fmt.Errorf("%w: already dual-signed", domain.ErrSignoffIncomplete)
	}
	if form.State == domain.HandoverStateVoided {
		return nil, fmt.Errorf("%w: form voided", domain.ErrInvalidTransition)
	}
	nextState := domain.HandoverNextStateAfterSignoff(form.State, req.Party)
	if nextState == form.State {
		return nil, fmt.Errorf("%w: party %s cannot sign in state %s", domain.ErrInvalidTransition, req.Party, form.State)
	}
	if err := domain.ValidateHandoverTransition(form.State, nextState); err != nil {
		return nil, fmt.Errorf("validate transition: %w", err)
	}
	prevState := form.State
	form.State = nextState
	form.Version++
	form.UpdatedAt = s.clock.Now()
	if req.Party == domain.SignoffPartyOutbound {
		form.OutboundSigner = req.Signer
		form.OutboundSignedAt = s.clock.Now()
	} else {
		form.ArrivalSigner = req.Signer
		form.ArrivalSignedAt = s.clock.Now()
	}
	if err := s.store.HandoverRepo().Save(ctx, form); err != nil {
		return nil, fmt.Errorf("save handover after signoff: %w", err)
	}
	sharedRecordLedger(ctx, s.store, s.clock, form.ShardID, form.FormNo, "",
		req.Signer, domain.LedgerEntryTypeSignoff, prevState, nextState)
	sharedAppendAudit(ctx, s.store, s.clock, req.Signer, "signoff_"+req.Party, domain.EntityTypeHandover, form.ID, form.ShardID, prevState, nextState, "")
	sharedPublishEvent(ctx, s.eventBus, s.clock, domain.EventHandoverSigned, form.ID, form.ShardID, form)
	return form, nil
}

func (s *HandoverService) Get(ctx context.Context, id string) (*domain.HandoverForm, error) {
	return s.store.HandoverRepo().Get(ctx, id)
}

func (s *HandoverService) List(ctx context.Context, filter storage.HandoverFilter) ([]*domain.HandoverForm, int, error) {
	return s.store.HandoverRepo().List(ctx, filter)
}

func (s *HandoverService) ModifySave(ctx context.Context, form *domain.HandoverForm) error {
	return s.store.HandoverRepo().Save(ctx, form)
}

var signoffMu sync.Mutex

func (s *HandoverService) SignoffWithLock(ctx context.Context, req domain.HandoverSignoffRequest) (*domain.HandoverForm, error) {
	signoffMu.Lock()
	defer signoffMu.Unlock()
	return s.Signoff(ctx, req)
}
