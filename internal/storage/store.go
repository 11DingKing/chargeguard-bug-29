package storage

import (
	"chargeguard/internal/auth"
	"chargeguard/internal/charging"
	"chargeguard/internal/domain"
	"context"
	"time"
)

type ShardMeta struct {
	ShardID     string    `json:"shard_id"`
	Date        string    `json:"date"`
	RouteID     string    `json:"route_id"`
	FilePath    string    `json:"file_path"`
	RecordCount int       `json:"record_count"`
	Checksum    string    `json:"checksum"`
	Status      string    `json:"status"`
	DataVersion int       `json:"data_version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

const (
	ShardStatusOK      = "ok"
	ShardStatusDamaged = "damaged"
)

type MailFilter struct {
	State      string
	RouteID    string
	VehicleNo  string
	StartTime  time.Time
	EndTime    time.Time
	PageSize   int
	PageOffset int
}

type HandoverFilter struct {
	State      string
	RouteID    string
	Date       string
	PageSize   int
	PageOffset int
}

type DispositionFilter struct {
	State       string
	MailID      string
	SubmittedBy string
	PageSize    int
	PageOffset  int
}

type MailItemRepository interface {
	Get(ctx context.Context, id string) (*domain.MailItem, error)
	GetByMailNo(ctx context.Context, mailNo string) (*domain.MailItem, error)
	Save(ctx context.Context, m *domain.MailItem) error
	List(ctx context.Context, filter MailFilter) ([]*domain.MailItem, int, error)
	UpdateStateTx(ctx context.Context, tx Tx, id, fromState, toState string, version int) error
}

type HandoverRepository interface {
	Get(ctx context.Context, id string) (*domain.HandoverForm, error)
	GetByFormNo(ctx context.Context, formNo string) (*domain.HandoverForm, error)
	Save(ctx context.Context, h *domain.HandoverForm) error
	List(ctx context.Context, filter HandoverFilter) ([]*domain.HandoverForm, int, error)
}

type DispositionRepository interface {
	Get(ctx context.Context, id string) (*domain.DispositionRequest, error)
	Save(ctx context.Context, d *domain.DispositionRequest) error
	GetActiveByMail(ctx context.Context, mailID string) ([]*domain.DispositionRequest, error)
	List(ctx context.Context, filter DispositionFilter) ([]*domain.DispositionRequest, int, error)
	CountActiveByMailTx(ctx context.Context, tx Tx, mailID string) (int, error)
	SaveTx(ctx context.Context, tx Tx, d *domain.DispositionRequest) error
}

type BatchRepository interface {
	Get(ctx context.Context, id string) (*domain.BatchRecord, error)
	Save(ctx context.Context, b *domain.BatchRecord) error
	GetItem(ctx context.Context, itemID string) (*domain.BatchItem, error)
	SaveItem(ctx context.Context, item *domain.BatchItem) error
	ListItems(ctx context.Context, batchID string) ([]*domain.BatchItem, error)
	ListPending(ctx context.Context) ([]*domain.BatchRecord, error)
}

type LedgerRepository interface {
	Save(ctx context.Context, e *domain.LedgerEntry) error
	List(ctx context.Context, q domain.LedgerQuery) ([]*domain.LedgerEntry, int, error)
	ListByVolume(ctx context.Context, date, routeID string) ([]*domain.LedgerEntry, error)
}

type EventRepository interface {
	Append(ctx context.Context, e *domain.Event) (int64, error)
	ListAfter(ctx context.Context, afterID int64, limit int) ([]*domain.Event, error)
	GetLastID(ctx context.Context) (int64, error)
	Prune(ctx context.Context, before time.Time) (int, error)
}

type SubscriberRepository interface {
	Get(ctx context.Context, id string) (*domain.Subscriber, error)
	Save(ctx context.Context, s *domain.Subscriber) error
	UpdateCheckpoint(ctx context.Context, id string, lastEventID int64) error
}

type AuditRepository interface {
	Append(ctx context.Context, r *domain.AuditRecord) error
	List(ctx context.Context, q domain.AuditQuery) ([]*domain.AuditRecord, int, error)
}

type FailureRepository interface {
	Save(ctx context.Context, f *domain.PermanentFailure) error
	Get(ctx context.Context, id int64) (*domain.PermanentFailure, error)
	List(ctx context.Context, q domain.FailureQuery) ([]*domain.PermanentFailure, int, error)
	Update(ctx context.Context, f *domain.PermanentFailure) error
	ListPending(ctx context.Context) ([]*domain.PermanentFailure, error)
}

type ShardMetaRepository interface {
	Get(ctx context.Context, shardID string) (*ShardMeta, error)
	Save(ctx context.Context, m *ShardMeta) error
	ListDamaged(ctx context.Context) ([]*ShardMeta, error)
	ListAll(ctx context.Context) ([]*ShardMeta, error)
}

type Tx interface {
	Commit() error
	Rollback() error
}

type Store interface {
	auth.Repository
	charging.Repository
	MailItemRepo() MailItemRepository
	HandoverRepo() HandoverRepository
	DispositionRepo() DispositionRepository
	BatchRepo() BatchRepository
	LedgerRepo() LedgerRepository
	EventRepo() EventRepository
	SubscriberRepo() SubscriberRepository
	AuditRepo() AuditRepository
	FailureRepo() FailureRepository
	ShardRepo() ShardMetaRepository
	BeginTx(ctx context.Context) (Tx, error)
	Close() error
	Ping(ctx context.Context) error
	DataDir() string
}

func (s *sqliteStore) ShardExists(shardID string) bool {
	return s.shard.Exists(shardID)
}

func (s *sqliteStore) ShardChecksum(shardID string) (string, int, error) {
	return s.shard.Checksum(shardID)
}
