package storage

import (
	"chargeguard/internal/domain"
	"context"
	"database/sql"
	"fmt"
	"time"
)

type dispositionRepo struct {
	store *sqliteStore
}

const dispositionColumns = "id, request_no, mail_id, mail_no, type, target_address, state, submitted_by, submitted_at, reviewed_by, reviewed_at, review_note, issued_by, issued_at, executed_at, completed_at, withdrawn_by, withdrawn_at, withdrawn_reason, conflict_reason, lost_at, version, shard_id, data_version"

func (r *dispositionRepo) Get(ctx context.Context, id string) (*domain.DispositionRequest, error) {
	row := r.store.db.QueryRowContext(ctx,
		`SELECT `+dispositionColumns+` FROM disposition_requests WHERE id = ?`, id)
	return scanDisposition(row)
}

func (r *dispositionRepo) Save(ctx context.Context, d *domain.DispositionRequest) error {
	tx, err := r.store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()
	if err := r.SaveTx(ctx, &sqliteTx{tx: tx}, d); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	r.store.appendShardRecord(d.ShardID, "disposition", d)
	return nil
}

func (r *dispositionRepo) SaveTx(ctx context.Context, tx Tx, d *domain.DispositionRequest) error {
	sqtx := tx.(*sqliteTx).tx
	if err := r.saveTx(ctx, sqtx, d); err != nil {
		return err
	}
	if domain.IsDispositionActive(d.State) {
		if err := r.upsertActiveDispTx(ctx, sqtx, d); err != nil {
			return err
		}
	} else {
		if err := r.removeActiveDispTx(ctx, sqtx, d.MailID, d.ID); err != nil {
			return err
		}
	}
	return r.store.ensureShardMetaTx(ctx, sqtx, d.ShardID, "")
}

func (r *dispositionRepo) saveTx(ctx context.Context, tx *sql.Tx, d *domain.DispositionRequest) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO disposition_requests (id, request_no, mail_id, mail_no, type, target_address, state,
			submitted_by, submitted_at, reviewed_by, reviewed_at, review_note, issued_by, issued_at,
			executed_at, completed_at, withdrawn_by, withdrawn_at, withdrawn_reason, conflict_reason,
			lost_at, version, shard_id, data_version)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET
			request_no=excluded.request_no, mail_id=excluded.mail_id, mail_no=excluded.mail_no,
			type=excluded.type, target_address=excluded.target_address, state=excluded.state,
			submitted_by=excluded.submitted_by, submitted_at=excluded.submitted_at,
			reviewed_by=excluded.reviewed_by, reviewed_at=excluded.reviewed_at,
			review_note=excluded.review_note, issued_by=excluded.issued_by, issued_at=excluded.issued_at,
			executed_at=excluded.executed_at, completed_at=excluded.completed_at,
			withdrawn_by=excluded.withdrawn_by, withdrawn_at=excluded.withdrawn_at,
			withdrawn_reason=excluded.withdrawn_reason, conflict_reason=excluded.conflict_reason,
			lost_at=excluded.lost_at, version=excluded.version, shard_id=excluded.shard_id,
			data_version=excluded.data_version`,
		d.ID, d.RequestNo, d.MailID, d.MailNo, d.Type, d.TargetAddress, d.State,
		d.SubmittedBy, d.SubmittedAt.Format(time.RFC3339), d.ReviewedBy,
		formatTime(d.ReviewedAt), d.ReviewNote, d.IssuedBy, formatTime(d.IssuedAt),
		formatTime(d.ExecutedAt), formatTime(d.CompletedAt), d.WithdrawnBy,
		formatTime(d.WithdrawnAt), d.WithdrawnReason, d.ConflictReason, formatTime(d.LostAt),
		d.Version, d.ShardID, d.DataVersion)
	if err != nil {
		return fmt.Errorf("upsert disposition: %w", err)
	}
	return nil
}

func (r *dispositionRepo) upsertActiveDispTx(ctx context.Context, tx *sql.Tx, d *domain.DispositionRequest) error {
	now := r.store.clock.Now().Format(time.RFC3339)
	_, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO active_dispositions (mail_id, disposition_id, created_at)
		 VALUES (?,?,?)`,
		d.MailID, d.ID, now)
	return err
}

func (r *dispositionRepo) removeActiveDispTx(ctx context.Context, tx *sql.Tx, mailID, dispID string) error {
	_, err := tx.ExecContext(ctx, "DELETE FROM active_dispositions WHERE mail_id = ? AND disposition_id = ?", mailID, dispID)
	return err
}

func (r *dispositionRepo) CountActiveByMailTx(ctx context.Context, tx Tx, mailID string) (int, error) {
	sqtx := tx.(*sqliteTx).tx
	var count int
	err := sqtx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM active_dispositions WHERE mail_id = ?", mailID).Scan(&count)
	return count, err
}

func (r *dispositionRepo) GetActiveByMail(ctx context.Context, mailID string) ([]*domain.DispositionRequest, error) {
	rows, err := r.store.db.QueryContext(ctx,
		`SELECT d.id, d.request_no, d.mail_id, d.mail_no, d.type, d.target_address, d.state,
			d.submitted_by, d.submitted_at, d.reviewed_by, d.reviewed_at, d.review_note,
			d.issued_by, d.issued_at, d.executed_at, d.completed_at, d.withdrawn_by, d.withdrawn_at,
			d.withdrawn_reason, d.conflict_reason, d.lost_at, d.version, d.shard_id, d.data_version
		 FROM disposition_requests d
		 JOIN active_dispositions a ON a.disposition_id = d.id
		 WHERE a.mail_id = ?`, mailID)
	if err != nil {
		return nil, fmt.Errorf("query active dispositions: %w", err)
	}
	defer rows.Close()
	var items []*domain.DispositionRequest
	for rows.Next() {
		d, err := scanDisposition(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, d)
	}
	return items, nil
}

func (r *dispositionRepo) List(ctx context.Context, filter DispositionFilter) ([]*domain.DispositionRequest, int, error) {
	var w whereBuilder
	w.eq("state", filter.State)
	w.eq("mail_id", filter.MailID)
	w.eq("submitted_by", filter.SubmittedBy)
	return queryPaged(ctx, r.store.db, "disposition_requests", dispositionColumns, "submitted_at DESC", w.clause(), w.args, filter.PageSize, filter.PageOffset, scanDisposition)
}

func scanDisposition(s rowScanner) (*domain.DispositionRequest, error) {
	var d domain.DispositionRequest
	var submittedAt, reviewedAt, issuedAt, executedAt, completedAt, withdrawnAt, lostAt string
	err := s.Scan(
		&d.ID, &d.RequestNo, &d.MailID, &d.MailNo, &d.Type, &d.TargetAddress, &d.State,
		&d.SubmittedBy, &submittedAt, &d.ReviewedBy, &reviewedAt, &d.ReviewNote,
		&d.IssuedBy, &issuedAt, &executedAt, &completedAt, &d.WithdrawnBy, &withdrawnAt,
		&d.WithdrawnReason, &d.ConflictReason, &lostAt, &d.Version, &d.ShardID, &d.DataVersion)
	if err != nil {
		return nil, wrapScanErr(err, "disposition")
	}
	d.SubmittedAt = parseTime(submittedAt)
	d.ReviewedAt = parseTime(reviewedAt)
	d.IssuedAt = parseTime(issuedAt)
	d.ExecutedAt = parseTime(executedAt)
	d.CompletedAt = parseTime(completedAt)
	d.WithdrawnAt = parseTime(withdrawnAt)
	d.LostAt = parseTime(lostAt)
	return &d, nil
}
