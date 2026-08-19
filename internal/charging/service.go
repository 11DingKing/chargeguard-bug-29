package charging

import (
	"chargeguard/internal/auth"
	"context"
	"fmt"
	"time"
)

type Repository interface {
	CreateStation(context.Context, *Station) error
	GetStation(context.Context, string) (*Station, error)
	ListStations(context.Context, int, int) ([]Station, int, error)
	CreateHazard(context.Context, *Hazard) error
	GetHazard(context.Context, string) (*Hazard, error)
	TransitionHazard(context.Context, *Hazard, HazardState, string, string) error
	CreateInspection(context.Context, *Inspection) error
	ListOpenHazards(context.Context, string, int, int) ([]Hazard, int, error)
}

type Service struct {
	repo Repository
	auth *auth.Service
	now  func() time.Time
}

func NewService(repo Repository, authService *auth.Service, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, auth: authService, now: now}
}

func (s *Service) CreateStation(ctx context.Context, token string, station Station) (Station, error) {
	if _, err := s.auth.RequireRole(ctx, token, auth.RoleRegulator, auth.RoleProsecutor); err != nil {
		return Station{}, err
	}
	if station.ID == "" || station.Name == "" || station.OperatorID == "" {
		return Station{}, fmt.Errorf("station fields: %w", ErrInvalidState)
	}
	station.Status = StationActive
	station.CreatedAt = s.now()
	station.UpdatedAt = station.CreatedAt
	station.Version = 1
	if err := s.repo.CreateStation(ctx, &station); err != nil {
		return Station{}, err
	}
	return station, nil
}

func (s *Service) ReportHazard(ctx context.Context, token string, hazard Hazard) (Hazard, error) {
	session, err := s.auth.RequireRole(ctx, token, auth.RoleInspector, auth.RoleProsecutor)
	if err != nil {
		return Hazard{}, err
	}
	if _, err := s.repo.GetStation(ctx, hazard.StationID); err != nil {
		return Hazard{}, err
	}
	if hazard.ID == "" || hazard.Kind == "" || hazard.Description == "" {
		return Hazard{}, fmt.Errorf("hazard fields: %w", ErrInvalidState)
	}
	hazard.ReportedBy = session.UserID
	hazard.State = HazardOpen
	hazard.CreatedAt = s.now()
	hazard.UpdatedAt = hazard.CreatedAt
	hazard.Version = 1
	if hazard.DueAt.IsZero() {
		hazard.DueAt = s.now().Add(48 * time.Hour)
	}
	if err := s.repo.CreateHazard(ctx, &hazard); err != nil {
		return Hazard{}, err
	}
	return hazard, nil
}

func (s *Service) AssignHazard(ctx context.Context, token, id, operatorID string) error {
	if _, err := s.auth.RequireRole(ctx, token, auth.RoleRegulator, auth.RoleProsecutor); err != nil {
		return err
	}
	h, err := s.repo.GetHazard(ctx, id)
	if err != nil {
		return err
	}
	if !h.CanTransition(HazardAssigned) {
		return ErrInvalidState
	}
	return s.repo.TransitionHazard(ctx, h, HazardAssigned, operatorID, "")
}

func (s *Service) RectifyHazard(ctx context.Context, token, id, evidence string) error {
	session, err := s.auth.RequireRole(ctx, token, auth.RoleOperator)
	if err != nil {
		return err
	}
	if evidence == "" {
		return ErrMissingEvidence
	}
	h, err := s.repo.GetHazard(ctx, id)
	if err != nil {
		return err
	}
	if h.AssignedTo != session.UserID && h.AssignedTo != "" {
		return auth.ErrForbidden
	}
	if !h.CanTransition(HazardRectified) {
		return ErrInvalidState
	}
	return s.repo.TransitionHazard(ctx, h, HazardRectified, session.UserID, evidence)
}

func (s *Service) VerifyHazard(ctx context.Context, token, id, evidence string) error {
	if _, err := s.auth.RequireRole(ctx, token, auth.RoleInspector, auth.RoleRegulator); err != nil {
		return err
	}
	if evidence == "" {
		return ErrMissingEvidence
	}
	h, err := s.repo.GetHazard(ctx, id)
	if err != nil {
		return err
	}
	if !h.CanTransition(HazardVerified) {
		return ErrInvalidState
	}
	return s.repo.TransitionHazard(ctx, h, HazardVerified, "", evidence)
}
