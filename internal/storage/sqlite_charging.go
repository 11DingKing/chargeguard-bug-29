package storage

import (
	"chargeguard/internal/charging"
	"chargeguard/internal/domain"
	"context"
	"database/sql"
	"errors"
	"time"
)

func (s *sqliteStore) CreateStation(ctx context.Context, station *charging.Station) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO charging_stations(id,name,county,operator_id,status,latitude,longitude,created_at,updated_at,version) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		station.ID, station.Name, station.County, station.OperatorID, station.Status, station.Latitude, station.Longitude, station.CreatedAt.Format(time.RFC3339Nano), station.UpdatedAt.Format(time.RFC3339Nano), station.Version)
	return err
}
func (s *sqliteStore) GetStation(ctx context.Context, id string) (*charging.Station, error) {
	var v charging.Station
	var created, updated string
	err := s.db.QueryRowContext(ctx, `SELECT id,name,county,operator_id,status,latitude,longitude,created_at,updated_at,version FROM charging_stations WHERE id=?`, id).Scan(&v.ID, &v.Name, &v.County, &v.OperatorID, &v.Status, &v.Latitude, &v.Longitude, &created, &updated, &v.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, charging.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	v.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	v.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return &v, nil
}
func (s *sqliteStore) ListStations(ctx context.Context, limit, offset int) ([]charging.Station, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM charging_stations`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,county,operator_id,status,latitude,longitude,created_at,updated_at,version FROM charging_stations ORDER BY name LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []charging.Station{}
	for rows.Next() {
		var v charging.Station
		var created, updated string
		if err := rows.Scan(&v.ID, &v.Name, &v.County, &v.OperatorID, &v.Status, &v.Latitude, &v.Longitude, &created, &updated, &v.Version); err != nil {
			return nil, 0, err
		}
		v.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		v.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		out = append(out, v)
	}
	return out, total, rows.Err()
}
func (s *sqliteStore) CreateHazard(ctx context.Context, h *charging.Hazard) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO charging_hazards(id,station_id,kind,severity,description,state,reported_by,assigned_to,due_at,evidence,version,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, h.ID, h.StationID, h.Kind, h.Severity, h.Description, h.State, h.ReportedBy, h.AssignedTo, h.DueAt.Format(time.RFC3339Nano), h.Evidence, h.Version, h.CreatedAt.Format(time.RFC3339Nano), h.UpdatedAt.Format(time.RFC3339Nano))
	return err
}
func (s *sqliteStore) GetHazard(ctx context.Context, id string) (*charging.Hazard, error) {
	var h charging.Hazard
	var due, created, updated string
	err := s.db.QueryRowContext(ctx, `SELECT id,station_id,kind,severity,description,state,reported_by,assigned_to,due_at,evidence,version,created_at,updated_at FROM charging_hazards WHERE id=?`, id).Scan(&h.ID, &h.StationID, &h.Kind, &h.Severity, &h.Description, &h.State, &h.ReportedBy, &h.AssignedTo, &due, &h.Evidence, &h.Version, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, charging.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	h.DueAt, _ = time.Parse(time.RFC3339Nano, due)
	h.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	h.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return &h, nil
}
func (s *sqliteStore) TransitionHazard(ctx context.Context, h *charging.Hazard, next charging.HazardState, assigned, evidence string) error {
	now := s.clock.Now()
	res, err := s.db.ExecContext(ctx, `UPDATE charging_hazards SET state=?,assigned_to=CASE WHEN ?='' THEN assigned_to ELSE ? END,evidence=CASE WHEN ?='' THEN evidence ELSE ? END,updated_at=?,version=version+1 WHERE id=? AND version=?`, next, assigned, assigned, evidence, evidence, now.Format(time.RFC3339Nano), h.ID, h.Version)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return domain.ErrConflict
	}
	return nil
}
func (s *sqliteStore) CreateInspection(ctx context.Context, i *charging.Inspection) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO charging_inspections(id,station_id,inspector_id,checked_at,extinguishers_ok,extinguisher_expiry,crash_barrier_ok,signage_ok,notes) VALUES(?,?,?,?,?,?,?,?,?)`, i.ID, i.StationID, i.InspectorID, i.CheckedAt.Format(time.RFC3339Nano), i.ExtinguishersOK, i.ExtinguisherExpiry.Format(time.RFC3339Nano), i.CrashBarrierOK, i.SignageOK, i.Notes)
	return err
}
func (s *sqliteStore) ListOpenHazards(ctx context.Context, stationID string, limit, offset int) ([]charging.Hazard, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM charging_hazards WHERE (?='' OR station_id=?) AND state<>'verified'`, stationID, stationID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,station_id,kind,severity,description,state,reported_by,assigned_to,due_at,evidence,version,created_at,updated_at FROM charging_hazards WHERE (?='' OR station_id=?) AND state<>'verified' ORDER BY due_at LIMIT ? OFFSET ?`, stationID, stationID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []charging.Hazard{}
	for rows.Next() {
		var h charging.Hazard
		var due, created, updated string
		if err := rows.Scan(&h.ID, &h.StationID, &h.Kind, &h.Severity, &h.Description, &h.State, &h.ReportedBy, &h.AssignedTo, &due, &h.Evidence, &h.Version, &created, &updated); err != nil {
			return nil, 0, err
		}
		h.DueAt, _ = time.Parse(time.RFC3339Nano, due)
		h.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		h.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		out = append(out, h)
	}
	return out, total, rows.Err()
}
