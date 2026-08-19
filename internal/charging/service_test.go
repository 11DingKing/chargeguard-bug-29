package charging

import (
	"chargeguard/internal/auth"
	"context"
	"testing"
	"time"
)

type fakeRepo struct {
	stations map[string]*Station
	hazards  map[string]*Hazard
}

func (f *fakeRepo) CreateStation(_ context.Context, s *Station) error {
	f.stations[s.ID] = s
	return nil
}
func (f *fakeRepo) GetStation(_ context.Context, id string) (*Station, error) {
	s, ok := f.stations[id]
	if !ok {
		return nil, ErrNotFound
	}
	return s, nil
}
func (f *fakeRepo) ListStations(context.Context, int, int) ([]Station, int, error) {
	return nil, 0, nil
}
func (f *fakeRepo) CreateHazard(_ context.Context, h *Hazard) error { f.hazards[h.ID] = h; return nil }
func (f *fakeRepo) GetHazard(_ context.Context, id string) (*Hazard, error) {
	h, ok := f.hazards[id]
	if !ok {
		return nil, ErrNotFound
	}
	return h, nil
}
func (f *fakeRepo) TransitionHazard(_ context.Context, h *Hazard, next HazardState, assigned, evidence string) error {
	h.State = next
	h.AssignedTo = assigned
	h.Evidence = evidence
	return nil
}
func (f *fakeRepo) CreateInspection(context.Context, *Inspection) error { return nil }
func (f *fakeRepo) ListOpenHazards(context.Context, string, int, int) ([]Hazard, int, error) {
	return nil, 0, nil
}

func TestHazardLifecycleRequiresEvidence(t *testing.T) {
	repo := &fakeRepo{stations: map[string]*Station{}, hazards: map[string]*Hazard{}}
	a := auth.New(time.Hour, time.Now)
	svc := NewService(repo, a, time.Now)
	ctx := context.Background()
	r, _ := a.Login(ctx, "regulator", "regulator-demo")
	o, _ := a.Login(ctx, "operator", "operator-demo")
	i, _ := a.Login(ctx, "inspector", "inspector-demo")
	if _, err := svc.CreateStation(ctx, r.ID, Station{ID: "st-1", Name: "人民路站", OperatorID: "u-operator"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ReportHazard(ctx, i.ID, Hazard{ID: "hz-1", StationID: "st-1", Kind: "expired_extinguisher", Description: "灭火器压力不足"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.AssignHazard(ctx, r.ID, "hz-1", o.UserID); err != nil {
		t.Fatal(err)
	}
	if err := svc.RectifyHazard(ctx, o.ID, "hz-1", ""); err != ErrMissingEvidence {
		t.Fatalf("got %v", err)
	}
	if err := svc.RectifyHazard(ctx, o.ID, "hz-1", "photo://hz-1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.VerifyHazard(ctx, i.ID, "hz-1", "check://hz-1"); err != nil {
		t.Fatal(err)
	}
}
