package storage

import (
	"chargeguard/internal/domain"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type RecoveryReport struct {
	TotalShards   int
	RebuiltShards int
	DamagedShards []string
	TotalRecords  int
	Errors        []string
}

type shardRecord struct {
	Type string          `json:"type"`
	TS   string          `json:"ts"`
	Data json.RawMessage `json:"data"`
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func RecoverFromShards(ctx context.Context, store Store) (*RecoveryReport, error) {
	report := &RecoveryReport{}
	sqlite := store.(*sqliteStore)
	shardsDir := filepath.Join(sqlite.dataDir, "shards")
	dateDirs, err := os.ReadDir(shardsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return report, nil
		}
		return nil, fmt.Errorf("read shards dir: %w", err)
	}
	for _, dateEntry := range dateDirs {
		if !dateEntry.IsDir() {
			continue
		}
		date := dateEntry.Name()
		routeFiles, err := os.ReadDir(filepath.Join(shardsDir, date))
		if err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("read date dir %s: %v", date, err))
			continue
		}
		for _, rf := range routeFiles {
			if rf.IsDir() {
				continue
			}
			routeID := rf.Name()
			if len(routeID) > 6 && routeID[len(routeID)-6:] == ".jsonl" {
				routeID = routeID[:len(routeID)-6]
			}
			shardID := domain.ShardIDFor(date, routeID)
			report.TotalShards++
			if err := recoverShard(ctx, sqlite, shardID, date, routeID, report); err != nil {
				report.Errors = append(report.Errors, err.Error())
			}
		}
	}
	return report, nil
}

func recoverShard(ctx context.Context, s *sqliteStore, shardID, date, routeID string, report *RecoveryReport) error {
	path := s.shard.shardPath(shardID)
	data, err := os.ReadFile(path)
	if err != nil {
		report.DamagedShards = append(report.DamagedShards, shardID)
		return fmt.Errorf("%w: read shard %s", domain.ErrShardCorrupted, shardID)
	}
	checksum := computeChecksum(data)
	now := s.clock.Now()
	lines := splitLines(data)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx for recovery: %w", err)
	}
	defer tx.Rollback()
	for _, line := range lines {
		var rec shardRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("unmarshal in %s: %v", shardID, err))
			continue
		}
		if err := replayRecord(ctx, tx, rec); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("replay %s in %s: %v", rec.Type, shardID, err))
			continue
		}
		report.TotalRecords++
	}
	meta := &ShardMeta{
		ShardID: shardID, Date: date, RouteID: routeID,
		FilePath: path, Checksum: checksum, Status: ShardStatusOK,
		DataVersion: 1, RecordCount: len(lines),
		CreatedAt: now, UpdatedAt: now,
	}
	if err := upsertShardMeta(ctx, tx, meta); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit recovery: %w", err)
	}
	report.RebuiltShards++
	return nil
}

func replayRecord(ctx context.Context, ex execer, rec shardRecord) error {
	switch rec.Type {
	case "mail":
		var m domain.MailItem
		if err := json.Unmarshal(rec.Data, &m); err != nil {
			return err
		}
		return replayMail(ctx, ex, &m)
	case "handover":
		var h domain.HandoverForm
		if err := json.Unmarshal(rec.Data, &h); err != nil {
			return err
		}
		return replayHandover(ctx, ex, &h)
	case "disposition":
		var d domain.DispositionRequest
		if err := json.Unmarshal(rec.Data, &d); err != nil {
			return err
		}
		return replayDisposition(ctx, ex, &d)
	case "ledger":
		var e domain.LedgerEntry
		if err := json.Unmarshal(rec.Data, &e); err != nil {
			return err
		}
		_, err := ex.ExecContext(ctx,
			`INSERT OR REPLACE INTO ledger_entries (id, date, route_id, volume_no, form_no, entry_type, mail_no, responsible, description, prev_state, next_state, created_at, shard_id, data_version)
			 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			e.ID, e.Date, e.RouteID, e.VolumeNo, e.FormNo, e.EntryType, e.MailNo,
			e.Responsible, e.Description, e.PrevState, e.NextState,
			e.CreatedAt.Format(time.RFC3339), e.ShardID, e.DataVersion)
		return err
	default:
		return nil
	}
}

func replayMail(ctx context.Context, ex execer, m *domain.MailItem) error {
	_, err := ex.ExecContext(ctx,
		`INSERT INTO mail_items (id, mail_no, route_id, vehicle_no, state, handover_id, disposition_id,
			origin_station, dest_station, sender_name, sender_addr, receiver_name, receiver_addr,
			responsible, registered_at, loaded_at, in_transit_at, arrived_at, signed_at, completed_at,
			version, shard_id, data_version)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET state=excluded.state, version=excluded.version,
			handover_id=excluded.handover_id, disposition_id=excluded.disposition_id`,
		m.ID, m.MailNo, m.RouteID, m.VehicleNo, m.State, m.HandoverID, m.DispositionID,
		m.OriginStation, m.DestStation, m.SenderName, m.SenderAddr, m.ReceiverName, m.ReceiverAddr,
		m.Responsible, m.RegisteredAt.Format(time.RFC3339), formatTime(m.LoadedAt),
		formatTime(m.InTransitAt), formatTime(m.ArrivedAt), formatTime(m.SignedAt),
		formatTime(m.CompletedAt), m.Version, m.ShardID, m.DataVersion)
	return err
}

func replayHandover(ctx context.Context, ex execer, h *domain.HandoverForm) error {
	_, err := ex.ExecContext(ctx,
		`INSERT INTO handover_forms (id, form_no, date, route_id, vehicle_no, state, outbound_station,
			outbound_signer, outbound_signed_at, arrival_station, arrival_signer, arrival_signed_at,
			mail_item_count, responsible, registered_at, updated_at, version, shard_id, data_version)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET state=excluded.state, version=excluded.version`,
		h.ID, h.FormNo, h.Date, h.RouteID, h.VehicleNo, h.State, h.OutboundStation, h.OutboundSigner,
		formatTime(h.OutboundSignedAt), h.ArrivalStation, h.ArrivalSigner, formatTime(h.ArrivalSignedAt),
		h.MailItemCount, h.Responsible, h.RegisteredAt.Format(time.RFC3339),
		h.UpdatedAt.Format(time.RFC3339), h.Version, h.ShardID, h.DataVersion)
	return err
}

func replayDisposition(ctx context.Context, ex execer, d *domain.DispositionRequest) error {
	_, err := ex.ExecContext(ctx,
		`INSERT INTO disposition_requests (id, request_no, mail_id, mail_no, type, target_address, state,
			submitted_by, submitted_at, reviewed_by, reviewed_at, review_note, issued_by, issued_at,
			executed_at, completed_at, withdrawn_by, withdrawn_at, withdrawn_reason, conflict_reason,
			lost_at, version, shard_id, data_version)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET state=excluded.state, version=excluded.version`,
		d.ID, d.RequestNo, d.MailID, d.MailNo, d.Type, d.TargetAddress, d.State,
		d.SubmittedBy, d.SubmittedAt.Format(time.RFC3339), d.ReviewedBy,
		formatTime(d.ReviewedAt), d.ReviewNote, d.IssuedBy, formatTime(d.IssuedAt),
		formatTime(d.ExecutedAt), formatTime(d.CompletedAt), d.WithdrawnBy,
		formatTime(d.WithdrawnAt), d.WithdrawnReason, d.ConflictReason, formatTime(d.LostAt),
		d.Version, d.ShardID, d.DataVersion)
	return err
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			if start < i {
				lines = append(lines, data[start:i])
			}
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
