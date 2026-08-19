package service

import (
	"chargeguard/internal/domain"
	"chargeguard/internal/storage"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type ImportExportService struct {
	handSvc *HandoverService
	clock   domain.Clock
}

func NewImportExportService(handSvc *HandoverService, clock domain.Clock) *ImportExportService {
	return &ImportExportService{handSvc: handSvc, clock: clock}
}

type ImportRowResult struct {
	RowNumber int    `json:"row_number"`
	Status    string `json:"status"`
	FormNo    string `json:"form_no"`
	Error     string `json:"error,omitempty"`
}

type ImportResult struct {
	TotalRows  int               `json:"total_rows"`
	Succeeded  int               `json:"succeeded"`
	Failed     int               `json:"failed"`
	RowResults []ImportRowResult `json:"row_results"`
}

func (s *ImportExportService) ImportHandoversCSV(ctx context.Context, reader io.Reader) (*ImportResult, error) {
	csvReader := csv.NewReader(reader)
	records, err := csvReader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read csv: %w", err)
	}
	result := &ImportResult{TotalRows: 0}
	if len(records) < 2 {
		return result, nil
	}
	header := records[0]
	headerMap := make(map[string]int)
	for i, h := range header {
		headerMap[strings.TrimSpace(h)] = i
	}
	for rowIdx, row := range records[1:] {
		result.TotalRows++
		rowNum := rowIdx + 2
		formNo := getCSVField(row, headerMap, "form_no")
		date := getCSVField(row, headerMap, "date")
		routeID := getCSVField(row, headerMap, "route_id")
		vehicleNo := getCSVField(row, headerMap, "vehicle_no")
		outboundStation := getCSVField(row, headerMap, "outbound_station")
		arrivalStation := getCSVField(row, headerMap, "arrival_station")
		responsible := getCSVField(row, headerMap, "responsible")
		mailNo := getCSVField(row, headerMap, "mail_no")
		senderName := getCSVField(row, headerMap, "sender_name")
		senderAddr := getCSVField(row, headerMap, "sender_addr")
		receiverName := getCSVField(row, headerMap, "receiver_name")
		receiverAddr := getCSVField(row, headerMap, "receiver_addr")
		if formNo == "" || date == "" || routeID == "" {
			result.Failed++
			result.RowResults = append(result.RowResults, ImportRowResult{
				RowNumber: rowNum, Status: "failed", FormNo: formNo, Error: "missing required fields",
			})
			continue
		}
		req := RegisterHandoverRequest{
			FormNo: formNo, Date: date, RouteID: routeID, VehicleNo: vehicleNo,
			OutboundStation: outboundStation, ArrivalStation: arrivalStation, Responsible: responsible,
			MailItems: []RegisterMailItem{{
				MailNo: mailNo, SenderName: senderName, SenderAddr: senderAddr,
				ReceiverName: receiverName, ReceiverAddr: receiverAddr,
			}},
		}
		rr := s.registerRow(ctx, req, rowNum)
		if rr.Status == "failed" {
			result.Failed++
		} else {
			result.Succeeded++
		}
		result.RowResults = append(result.RowResults, rr)
	}
	return result, nil
}

func (s *ImportExportService) registerRow(ctx context.Context, req RegisterHandoverRequest, rowNum int) ImportRowResult {
	if _, err := s.handSvc.Register(ctx, req); err != nil {
		return ImportRowResult{RowNumber: rowNum, Status: "failed", FormNo: req.FormNo, Error: err.Error()}
	}
	return ImportRowResult{RowNumber: rowNum, Status: "succeeded", FormNo: req.FormNo}
}

func (s *ImportExportService) ImportHandoversJSON(ctx context.Context, reader io.Reader) (*ImportResult, error) {
	dec := json.NewDecoder(reader)
	var records []RegisterHandoverRequest
	if err := dec.Decode(&records); err != nil {
		return nil, fmt.Errorf("decode json: %w", err)
	}
	result := &ImportResult{TotalRows: len(records)}
	for i, req := range records {
		rr := s.registerRow(ctx, req, i+1)
		if rr.Status == "failed" {
			result.Failed++
		} else {
			result.Succeeded++
		}
		result.RowResults = append(result.RowResults, rr)
	}
	return result, nil
}

func getCSVField(row []string, headerMap map[string]int, field string) string {
	idx, ok := headerMap[field]
	if !ok || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

type MaintenanceService struct {
	store storage.Store
	clock domain.Clock
}

func NewMaintenanceService(store storage.Store, clock domain.Clock) *MaintenanceService {
	return &MaintenanceService{store: store, clock: clock}
}

type VerifyReport struct {
	TotalShards   int      `json:"total_shards"`
	OKShards      int      `json:"ok_shards"`
	DamagedShards int      `json:"damaged_shards"`
	Errors        []string `json:"errors"`
}

type shardChecker interface {
	ShardExists(shardID string) bool
	ShardChecksum(shardID string) (string, int, error)
}

func (s *MaintenanceService) Verify(ctx context.Context) (*VerifyReport, error) {
	report := &VerifyReport{}
	shards, err := s.store.ShardRepo().ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list shards: %w", err)
	}
	checker, ok := s.store.(shardChecker)
	if !ok {
		return nil, fmt.Errorf("store does not support shard checking")
	}
	report.TotalShards = len(shards)
	for _, shard := range shards {
		if shard.Status == storage.ShardStatusDamaged {
			report.DamagedShards++
			continue
		}
		if !checker.ShardExists(shard.ShardID) {
			report.DamagedShards++
			report.Errors = append(report.Errors, fmt.Sprintf("shard %s file missing", shard.ShardID))
			continue
		}
		checksum, _, err := checker.ShardChecksum(shard.ShardID)
		if err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("shard %s checksum error: %v", shard.ShardID, err))
			report.DamagedShards++
			continue
		}
		if shard.Checksum != "" && shard.Checksum != checksum {
			report.Errors = append(report.Errors, fmt.Sprintf("shard %s checksum mismatch", shard.ShardID))
			report.DamagedShards++
			continue
		}
		report.OKShards++
	}
	return report, nil
}

func (s *MaintenanceService) RebuildIndex(ctx context.Context) (*storage.RecoveryReport, error) {
	return storage.RecoverFromShards(ctx, s.store)
}

type BatchMaintenanceResult struct {
	TotalChecked int      `json:"total_checked"`
	Updated      int      `json:"updated"`
	Skipped      int      `json:"skipped"`
	Errors       []string `json:"errors"`
}

func (s *MaintenanceService) BatchVerifyMailItems(ctx context.Context, mailIDs []string) (*BatchMaintenanceResult, error) {
	result := &BatchMaintenanceResult{}
	for _, id := range mailIDs {
		result.TotalChecked++
		mail, err := s.store.MailItemRepo().Get(ctx, id)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("mail %s: %v", id, err))
			continue
		}
		if mail.State == "" || mail.MailNo == "" {
			result.Errors = append(result.Errors, fmt.Sprintf("mail %s: invalid data", id))
			continue
		}
		if mail.Version <= 0 {
			mail.Version = 1
			mail.DataVersion = 1
			if err := s.store.MailItemRepo().Save(ctx, mail); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("mail %s: save failed: %v", id, err))
				continue
			}
			result.Updated++
			continue
		}
		result.Skipped++
	}
	return result, nil
}
