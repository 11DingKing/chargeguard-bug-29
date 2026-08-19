package storage

import (
	"chargeguard/internal/domain"
	"context"
	"database/sql"
	"fmt"
	"time"
)

type batchRepo struct {
	store *sqliteStore
}

const batchColumns = "id, vehicle_no, date, route_id, state, total_count, succeeded_count, failed_count, created_at, updated_at, version, shard_id, data_version"

func (r *batchRepo) Get(ctx context.Context, id string) (*domain.BatchRecord, error) {
	row := r.store.db.QueryRowContext(ctx,
		`SELECT `+batchColumns+` FROM batch_records WHERE id = ?`, id)
	return scanBatch(row)
}

func (r *batchRepo) Save(ctx context.Context, b *domain.BatchRecord) error {
	tx, err := r.store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx,
		`INSERT INTO batch_records (id, vehicle_no, date, route_id, state, total_count, succeeded_count,
			failed_count, created_at, updated_at, version, shard_id, data_version)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET
			vehicle_no=excluded.vehicle_no, date=excluded.date, route_id=excluded.route_id,
			state=excluded.state, total_count=excluded.total_count, succeeded_count=excluded.succeeded_count,
			failed_count=excluded.failed_count, updated_at=excluded.updated_at, version=excluded.version,
			shard_id=excluded.shard_id, data_version=excluded.data_version`,
		b.ID, b.VehicleNo, b.Date, b.RouteID, b.State, b.TotalCount, b.SucceededCount,
		b.FailedCount, b.CreatedAt.Format(time.RFC3339), b.UpdatedAt.Format(time.RFC3339),
		b.Version, b.ShardID, b.DataVersion)
	if err != nil {
		return fmt.Errorf("upsert batch: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

const batchItemColumns = "id, batch_id, mail_id, mail_no, state, error, created_at, updated_at"

func (r *batchRepo) GetItem(ctx context.Context, itemID string) (*domain.BatchItem, error) {
	row := r.store.db.QueryRowContext(ctx,
		`SELECT `+batchItemColumns+` FROM batch_items WHERE id = ?`, itemID)
	return scanBatchItem(row)
}

func (r *batchRepo) SaveItem(ctx context.Context, item *domain.BatchItem) error {
	_, err := r.store.db.ExecContext(ctx,
		`INSERT INTO batch_items (id, batch_id, mail_id, mail_no, state, error, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET
			batch_id=excluded.batch_id, mail_id=excluded.mail_id, mail_no=excluded.mail_no,
			state=excluded.state, error=excluded.error, updated_at=excluded.updated_at`,
		item.ID, item.BatchID, item.MailID, item.MailNo, item.State, item.Error,
		item.CreatedAt.Format(time.RFC3339), item.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("upsert batch item: %w", err)
	}
	return nil
}

func (r *batchRepo) ListItems(ctx context.Context, batchID string) ([]*domain.BatchItem, error) {
	return queryList(ctx, r.store.db, "batch items",
		`SELECT `+batchItemColumns+` FROM batch_items WHERE batch_id = ? ORDER BY created_at`,
		[]any{batchID}, scanBatchItem)
}

func (r *batchRepo) ListPending(ctx context.Context) ([]*domain.BatchRecord, error) {
	return queryList(ctx, r.store.db, "pending batches",
		`SELECT `+batchColumns+` FROM batch_records WHERE state IN ('pending','rolled_back') ORDER BY created_at`,
		nil, scanBatch)
}

type ledgerRepo struct {
	store *sqliteStore
}

func (r *ledgerRepo) Save(ctx context.Context, e *domain.LedgerEntry) error {
	return r.store.persistWithShard(ctx, e.ShardID, "", "ledger", e, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO ledger_entries (id, date, route_id, volume_no, form_no, entry_type, mail_no,
				responsible, description, prev_state, next_state, created_at, shard_id, data_version)
			 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			e.ID, e.Date, e.RouteID, e.VolumeNo, e.FormNo, e.EntryType, e.MailNo,
			e.Responsible, e.Description, e.PrevState, e.NextState,
			e.CreatedAt.Format(time.RFC3339), e.ShardID, e.DataVersion)
		if err != nil {
			return fmt.Errorf("insert ledger: %w", err)
		}
		return nil
	})
}

const ledgerColumns = "id, date, route_id, volume_no, form_no, entry_type, mail_no, responsible, description, prev_state, next_state, created_at, shard_id, data_version"

func (r *ledgerRepo) List(ctx context.Context, q domain.LedgerQuery) ([]*domain.LedgerEntry, int, error) {
	var w whereBuilder
	w.eq("date", q.Date)
	w.eq("route_id", q.RouteID)
	w.eq("responsible", q.Responsible)
	w.eq("entry_type", q.EntryType)
	w.since("created_at", q.StartTime)
	w.until("created_at", q.EndTime)
	return queryPaged(ctx, r.store.db, "ledger_entries", ledgerColumns, "created_at DESC", w.clause(), w.args, q.PageSize, q.PageOffset, scanLedger)
}

func (r *ledgerRepo) ListByVolume(ctx context.Context, date, routeID string) ([]*domain.LedgerEntry, error) {
	return queryList(ctx, r.store.db, "ledger by volume",
		`SELECT `+ledgerColumns+` FROM ledger_entries WHERE date = ? AND route_id = ? ORDER BY created_at`,
		[]any{date, routeID}, scanLedger)
}
