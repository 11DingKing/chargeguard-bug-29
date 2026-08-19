package service

import (
	"chargeguard/internal/domain"
	"chargeguard/internal/sla"
	"chargeguard/internal/storage"
	"context"
	"encoding/csv"
	"fmt"
	"strings"
	"time"
)

type LedgerService struct {
	store storage.Store
	clock domain.Clock
}

func NewLedgerService(store storage.Store, clock domain.Clock) *LedgerService {
	return &LedgerService{store: store, clock: clock}
}

func (s *LedgerService) Query(ctx context.Context, q domain.LedgerQuery) ([]*domain.LedgerEntry, int, error) {
	return s.store.LedgerRepo().List(ctx, q)
}

func (s *LedgerService) ExportCSV(ctx context.Context, date, routeID string) (string, error) {
	entries, err := s.store.LedgerRepo().ListByVolume(ctx, date, routeID)
	if err != nil {
		return "", fmt.Errorf("list ledger: %w", err)
	}
	var sb strings.Builder
	writer := csv.NewWriter(&sb)
	writer.Write([]string{"id", "date", "route_id", "volume_no", "form_no", "entry_type", "mail_no", "responsible", "description", "prev_state", "next_state", "created_at"})
	for _, e := range entries {
		writer.Write([]string{
			e.ID, e.Date, e.RouteID, e.VolumeNo, e.FormNo, e.EntryType,
			e.MailNo, e.Responsible, e.Description, e.PrevState, e.NextState,
			e.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}
	writer.Flush()
	return sb.String(), nil
}

func (s *LedgerService) ListVolumes(ctx context.Context, date string) ([]domain.LedgerVolume, error) {
	shards, err := s.store.ShardRepo().ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list shards: %w", err)
	}
	var volumes []domain.LedgerVolume
	for _, shard := range shards {
		if date != "" && shard.Date != date {
			continue
		}
		entries, err := s.store.LedgerRepo().ListByVolume(ctx, shard.Date, shard.RouteID)
		if err != nil {
			continue
		}
		volumes = append(volumes, domain.LedgerVolume{
			VolumeNo: shard.Date + "_" + shard.RouteID,
			Date:     shard.Date, RouteID: shard.RouteID,
			EntryCount: len(entries), Checksum: shard.Checksum,
		})
	}
	return volumes, nil
}

func (s *LedgerService) AuditQuery(ctx context.Context, q domain.AuditQuery) ([]*domain.AuditRecord, int, error) {
	return s.store.AuditRepo().List(ctx, q)
}

type OverdueService struct {
	store   storage.Store
	clock   domain.Clock
	timeout time.Duration
	sla     *sla.RuleSet
}

func NewOverdueService(store storage.Store, clock domain.Clock, timeout time.Duration) *OverdueService {
	return &OverdueService{store: store, clock: clock, timeout: timeout}
}

// WithSLA attaches a route-aware transit-SLA rule set so overdue reports
// also carry a per-route timeliness breakdown. When nil, reports omit it.
func (s *OverdueService) WithSLA(rules *sla.RuleSet) *OverdueService {
	s.sla = rules
	return s
}

type OverdueReport struct {
	OverdueMails   []*domain.MailItem    `json:"overdue_mails"`
	BacklogBatches []*domain.BatchRecord `json:"backlog_batches"`
	TotalOverdue   int                   `json:"total_overdue"`
	TotalBacklog   int                   `json:"total_backlog"`
	TimeoutHours   int                   `json:"timeout_hours"`
	SLAClasses     map[string]int        `json:"sla_classes,omitempty"`
}

func (s *OverdueService) GetOverdueReport(ctx context.Context) (*OverdueReport, error) {
	report := &OverdueReport{TimeoutHours: int(s.timeout.Hours())}
	cutoff := s.clock.Now().Add(-s.timeout)
	mails, _, err := s.store.MailItemRepo().List(ctx, storage.MailFilter{
		State:    domain.MailStateInTransit,
		EndTime:  cutoff,
		PageSize: 200,
	})
	if err != nil {
		return nil, fmt.Errorf("list overdue mails: %w", err)
	}
	report.OverdueMails = mails
	report.TotalOverdue = len(mails)
	if s.sla != nil {
		report.SLAClasses = s.sla.Summary(mails, s.clock.Now())
	}
	batches, err := s.store.BatchRepo().ListPending(ctx)
	if err != nil {
		return nil, fmt.Errorf("list backlog batches: %w", err)
	}
	report.BacklogBatches = batches
	report.TotalBacklog = len(batches)
	return report, nil
}

func (s *OverdueService) ListFailures(ctx context.Context, status string) ([]*domain.PermanentFailure, int, error) {
	return s.store.FailureRepo().List(ctx, domain.FailureQuery{Status: status, PageSize: 200})
}

func (s *OverdueService) ResolveFailure(ctx context.Context, failureID int64) error {
	f, err := s.store.FailureRepo().Get(ctx, failureID)
	if err != nil {
		return fmt.Errorf("get failure: %w", err)
	}
	f.Status = domain.FailureStatusResolved
	f.ResolvedAt = s.clock.Now()
	return s.store.FailureRepo().Update(ctx, f)
}
