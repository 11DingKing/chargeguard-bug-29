package storage

import (
	"chargeguard/internal/domain"
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"time"
)

//go:embed migrations/001_init.sql
var migrationSQL string

type sqliteStore struct {
	db         *sql.DB
	dataDir    string
	shard      *ShardWriter
	clock      domain.Clock
	mailRepo   *mailItemRepo
	handRepo   *handoverRepo
	dispRepo   *dispositionRepo
	batchRepo  *batchRepo
	ledgerRepo *ledgerRepo
	eventRepo  *eventRepo
	subRepo    *subscriberRepo
	auditRepo  *auditRepo
	failRepo   *failureRepo
	shardRepo  *shardMetaRepo
}

func NewStore(ctx context.Context, dataDir string, clock domain.Clock) (Store, error) {
	if clock == nil {
		clock = domain.RealClock{}
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	shardsDir := filepath.Join(dataDir, "shards")
	if err := os.MkdirAll(shardsDir, 0o755); err != nil {
		return nil, fmt.Errorf("create shards dir: %w", err)
	}
	dbPath := filepath.Join(dataDir, "manifest.db")
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &sqliteStore{
		db:      db,
		dataDir: dataDir,
		shard:   NewShardWriter(dataDir),
		clock:   clock,
	}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	s.initRepos()
	return s, nil
}

func (s *sqliteStore) initRepos() {
	s.mailRepo = &mailItemRepo{store: s}
	s.handRepo = &handoverRepo{store: s}
	s.dispRepo = &dispositionRepo{store: s}
	s.batchRepo = &batchRepo{store: s}
	s.ledgerRepo = &ledgerRepo{store: s}
	s.eventRepo = &eventRepo{store: s}
	s.subRepo = &subscriberRepo{store: s}
	s.auditRepo = &auditRepo{store: s}
	s.failRepo = &failureRepo{store: s}
	s.shardRepo = &shardMetaRepo{store: s}
}

func (s *sqliteStore) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, migrationSQL); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT OR IGNORE INTO schema_version(version, applied_at) VALUES(1, ?)",
		s.clock.Now().Format(time.RFC3339))
	return err
}

func (s *sqliteStore) BeginTx(ctx context.Context) (Tx, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	return &sqliteTx{tx: tx}, nil
}

func (s *sqliteStore) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *sqliteStore) Close() error {
	if err := s.shard.Close(); err != nil {
	}
	return s.db.Close()
}

func (s *sqliteStore) DataDir() string { return s.dataDir }

func (s *sqliteStore) MailItemRepo() MailItemRepository       { return s.mailRepo }
func (s *sqliteStore) HandoverRepo() HandoverRepository       { return s.handRepo }
func (s *sqliteStore) DispositionRepo() DispositionRepository { return s.dispRepo }
func (s *sqliteStore) BatchRepo() BatchRepository             { return s.batchRepo }
func (s *sqliteStore) LedgerRepo() LedgerRepository           { return s.ledgerRepo }
func (s *sqliteStore) EventRepo() EventRepository             { return s.eventRepo }
func (s *sqliteStore) SubscriberRepo() SubscriberRepository   { return s.subRepo }
func (s *sqliteStore) AuditRepo() AuditRepository             { return s.auditRepo }
func (s *sqliteStore) FailureRepo() FailureRepository         { return s.failRepo }
func (s *sqliteStore) ShardRepo() ShardMetaRepository         { return s.shardRepo }

type sqliteTx struct {
	tx *sql.Tx
}

func (t *sqliteTx) Commit() error   { return t.tx.Commit() }
func (t *sqliteTx) Rollback() error { return t.tx.Rollback() }

func (s *sqliteStore) ensureShardMetaTx(ctx context.Context, tx *sql.Tx, shardID, routeID string) error {
	date, _ := domain.SplitShardID(shardID)
	filePath := s.shard.shardPath(shardID)
	now := s.clock.Now().Format(time.RFC3339)
	_, err := tx.ExecContext(ctx,
		`INSERT INTO shard_meta (shard_id, date, route_id, file_path, record_count, checksum, status, data_version, created_at, updated_at)
		 VALUES (?,?,?,?,0,'','ok',1,?,?)
		 ON CONFLICT(shard_id) DO UPDATE SET updated_at = excluded.updated_at`,
		shardID, date, routeID, filePath, now, now)
	return err
}

func (s *sqliteStore) persistWithShard(ctx context.Context, shardID, routeID, kind string, record any, save func(context.Context, *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()
	if err := save(ctx, tx); err != nil {
		return err
	}
	if routeID != "" {
		if err := s.ensureShardMetaTx(ctx, tx, shardID, routeID); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	s.appendShardRecord(shardID, kind, record)
	return nil
}

func (s *sqliteStore) appendShardRecord(shardID, recordType string, data any) {
	wrapper := struct {
		Type string `json:"type"`
		TS   string `json:"ts"`
		Data any    `json:"data"`
	}{Type: recordType, TS: s.clock.Now().Format(time.RFC3339), Data: data}
	s.shard.Append(shardID, wrapper)
}
