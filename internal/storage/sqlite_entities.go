package storage

import (
	"chargeguard/internal/domain"
	"context"
	"database/sql"
	"fmt"
	"time"
)

type mailItemRepo struct {
	store *sqliteStore
}

func (r *mailItemRepo) Get(ctx context.Context, id string) (*domain.MailItem, error) {
	row := r.store.db.QueryRowContext(ctx,
		`SELECT `+mailColumns+` FROM mail_items WHERE id = ?`, id)
	return scanMail(row)
}

func (r *mailItemRepo) GetByMailNo(ctx context.Context, mailNo string) (*domain.MailItem, error) {
	row := r.store.db.QueryRowContext(ctx,
		`SELECT `+mailColumns+` FROM mail_items WHERE mail_no = ?`, mailNo)
	return scanMail(row)
}

func (r *mailItemRepo) Save(ctx context.Context, m *domain.MailItem) error {
	return r.store.persistWithShard(ctx, m.ShardID, m.RouteID, "mail", m, func(ctx context.Context, tx *sql.Tx) error {
		return r.saveTx(ctx, tx, m)
	})
}

func (r *mailItemRepo) saveTx(ctx context.Context, tx *sql.Tx, m *domain.MailItem) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO mail_items (id, mail_no, route_id, vehicle_no, state, handover_id, disposition_id,
			origin_station, dest_station, sender_name, sender_addr, receiver_name, receiver_addr,
			responsible, registered_at, loaded_at, in_transit_at, arrived_at, signed_at, completed_at,
			version, shard_id, data_version)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET
			mail_no=excluded.mail_no, route_id=excluded.route_id, vehicle_no=excluded.vehicle_no,
			state=excluded.state, handover_id=excluded.handover_id, disposition_id=excluded.disposition_id,
			origin_station=excluded.origin_station, dest_station=excluded.dest_station,
			sender_name=excluded.sender_name, sender_addr=excluded.sender_addr,
			receiver_name=excluded.receiver_name, receiver_addr=excluded.receiver_addr,
			responsible=excluded.responsible, loaded_at=excluded.loaded_at,
			in_transit_at=excluded.in_transit_at, arrived_at=excluded.arrived_at,
			signed_at=excluded.signed_at, completed_at=excluded.completed_at,
			version=excluded.version, shard_id=excluded.shard_id, data_version=excluded.data_version`,
		m.ID, m.MailNo, m.RouteID, m.VehicleNo, m.State, m.HandoverID, m.DispositionID,
		m.OriginStation, m.DestStation, m.SenderName, m.SenderAddr, m.ReceiverName, m.ReceiverAddr,
		m.Responsible, m.RegisteredAt.Format(time.RFC3339), formatTime(m.LoadedAt),
		formatTime(m.InTransitAt), formatTime(m.ArrivedAt), formatTime(m.SignedAt),
		formatTime(m.CompletedAt), m.Version, m.ShardID, m.DataVersion)
	if err != nil {
		return fmt.Errorf("upsert mail: %w", err)
	}
	return nil
}

func (r *mailItemRepo) UpdateStateTx(ctx context.Context, tx Tx, id, fromState, toState string, version int) error {
	sqtx := tx.(*sqliteTx).tx
	res, err := sqtx.ExecContext(ctx,
		`UPDATE mail_items SET state = ?, version = version + 1, in_transit_at = CASE WHEN ? = 'in_transit' THEN ? ELSE in_transit_at END,
			arrived_at = CASE WHEN ? = 'arrived' THEN ? ELSE arrived_at END,
			signed_at = CASE WHEN ? = 'dual_signed' THEN ? ELSE signed_at END,
			completed_at = CASE WHEN ? = 'completed' THEN ? ELSE completed_at END
		 WHERE id = ? AND state = ? AND version = ?`,
		toState, toState, r.store.clock.Now().Format(time.RFC3339),
		toState, r.store.clock.Now().Format(time.RFC3339),
		toState, r.store.clock.Now().Format(time.RFC3339),
		toState, r.store.clock.Now().Format(time.RFC3339),
		id, fromState, version)
	if err != nil {
		return fmt.Errorf("update mail state: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return domain.ConflictError{
			EntityID: id, Current: fromState, Attempted: toState,
			Reason: "state or version mismatch",
		}
	}
	return nil
}

const mailColumns = "id, mail_no, route_id, vehicle_no, state, handover_id, disposition_id, origin_station, dest_station, sender_name, sender_addr, receiver_name, receiver_addr, responsible, registered_at, loaded_at, in_transit_at, arrived_at, signed_at, completed_at, version, shard_id, data_version"

func (r *mailItemRepo) List(ctx context.Context, filter MailFilter) ([]*domain.MailItem, int, error) {
	var w whereBuilder
	w.eq("state", filter.State)
	w.eq("route_id", filter.RouteID)
	w.eq("vehicle_no", filter.VehicleNo)
	w.since("registered_at", filter.StartTime)
	w.until("registered_at", filter.EndTime)
	return queryPaged(ctx, r.store.db, "mail_items", mailColumns, "registered_at DESC", w.clause(), w.args, filter.PageSize, filter.PageOffset, scanMail)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanMail(s rowScanner) (*domain.MailItem, error) {
	var m domain.MailItem
	var registeredAt, loadedAt, inTransitAt, arrivedAt, signedAt, completedAt string
	err := s.Scan(
		&m.ID, &m.MailNo, &m.RouteID, &m.VehicleNo, &m.State, &m.HandoverID, &m.DispositionID,
		&m.OriginStation, &m.DestStation, &m.SenderName, &m.SenderAddr, &m.ReceiverName, &m.ReceiverAddr,
		&m.Responsible, &registeredAt, &loadedAt, &inTransitAt, &arrivedAt, &signedAt, &completedAt,
		&m.Version, &m.ShardID, &m.DataVersion)
	if err != nil {
		return nil, wrapScanErr(err, "mail")
	}
	m.RegisteredAt = parseTime(registeredAt)
	m.LoadedAt = parseTime(loadedAt)
	m.InTransitAt = parseTime(inTransitAt)
	m.ArrivedAt = parseTime(arrivedAt)
	m.SignedAt = parseTime(signedAt)
	m.CompletedAt = parseTime(completedAt)
	return &m, nil
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

type handoverRepo struct {
	store *sqliteStore
}

func (r *handoverRepo) Get(ctx context.Context, id string) (*domain.HandoverForm, error) {
	row := r.store.db.QueryRowContext(ctx,
		`SELECT id, form_no, date, route_id, vehicle_no, state, outbound_station, outbound_signer,
			outbound_signed_at, arrival_station, arrival_signer, arrival_signed_at, mail_item_count,
			responsible, registered_at, updated_at, version, shard_id, data_version
		 FROM handover_forms WHERE id = ?`, id)
	return scanHandover(row)
}

func (r *handoverRepo) GetByFormNo(ctx context.Context, formNo string) (*domain.HandoverForm, error) {
	row := r.store.db.QueryRowContext(ctx,
		`SELECT id, form_no, date, route_id, vehicle_no, state, outbound_station, outbound_signer,
			outbound_signed_at, arrival_station, arrival_signer, arrival_signed_at, mail_item_count,
			responsible, registered_at, updated_at, version, shard_id, data_version
		 FROM handover_forms WHERE form_no = ?`, formNo)
	return scanHandover(row)
}

func (r *handoverRepo) Save(ctx context.Context, h *domain.HandoverForm) error {
	return r.store.persistWithShard(ctx, h.ShardID, h.RouteID, "handover", h, func(ctx context.Context, tx *sql.Tx) error {
		return r.saveTx(ctx, tx, h)
	})
}

func (r *handoverRepo) saveTx(ctx context.Context, tx *sql.Tx, h *domain.HandoverForm) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO handover_forms (id, form_no, date, route_id, vehicle_no, state, outbound_station,
			outbound_signer, outbound_signed_at, arrival_station, arrival_signer, arrival_signed_at,
			mail_item_count, responsible, registered_at, updated_at, version, shard_id, data_version)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET
			form_no=excluded.form_no, date=excluded.date, route_id=excluded.route_id, vehicle_no=excluded.vehicle_no,
			state=excluded.state, outbound_station=excluded.outbound_station, outbound_signer=excluded.outbound_signer,
			outbound_signed_at=excluded.outbound_signed_at, arrival_station=excluded.arrival_station,
			arrival_signer=excluded.arrival_signer, arrival_signed_at=excluded.arrival_signed_at,
			mail_item_count=excluded.mail_item_count, responsible=excluded.responsible,
			updated_at=excluded.updated_at, version=excluded.version, shard_id=excluded.shard_id,
			data_version=excluded.data_version`,
		h.ID, h.FormNo, h.Date, h.RouteID, h.VehicleNo, h.State, h.OutboundStation, h.OutboundSigner,
		formatTime(h.OutboundSignedAt), h.ArrivalStation, h.ArrivalSigner, formatTime(h.ArrivalSignedAt),
		h.MailItemCount, h.Responsible, h.RegisteredAt.Format(time.RFC3339),
		h.UpdatedAt.Format(time.RFC3339), h.Version, h.ShardID, h.DataVersion)
	if err != nil {
		return fmt.Errorf("upsert handover: %w", err)
	}
	return nil
}

const handoverColumns = "id, form_no, date, route_id, vehicle_no, state, outbound_station, outbound_signer, outbound_signed_at, arrival_station, arrival_signer, arrival_signed_at, mail_item_count, responsible, registered_at, updated_at, version, shard_id, data_version"

func (r *handoverRepo) List(ctx context.Context, filter HandoverFilter) ([]*domain.HandoverForm, int, error) {
	var w whereBuilder
	w.eq("state", filter.State)
	w.eq("route_id", filter.RouteID)
	w.eq("date", filter.Date)
	return queryPaged(ctx, r.store.db, "handover_forms", handoverColumns, "registered_at DESC", w.clause(), w.args, filter.PageSize, filter.PageOffset, scanHandover)
}

func scanHandover(s rowScanner) (*domain.HandoverForm, error) {
	var h domain.HandoverForm
	var registeredAt, updatedAt, outSignedAt, arrSignedAt string
	err := s.Scan(
		&h.ID, &h.FormNo, &h.Date, &h.RouteID, &h.VehicleNo, &h.State,
		&h.OutboundStation, &h.OutboundSigner, &outSignedAt,
		&h.ArrivalStation, &h.ArrivalSigner, &arrSignedAt,
		&h.MailItemCount, &h.Responsible, &registeredAt, &updatedAt,
		&h.Version, &h.ShardID, &h.DataVersion)
	if err != nil {
		return nil, wrapScanErr(err, "handover")
	}
	h.RegisteredAt = parseTime(registeredAt)
	h.UpdatedAt = parseTime(updatedAt)
	h.OutboundSignedAt = parseTime(outSignedAt)
	h.ArrivalSignedAt = parseTime(arrSignedAt)
	return &h, nil
}
