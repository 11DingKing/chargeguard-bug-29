package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"chargeguard/internal/audit"
	"chargeguard/internal/config"
	"chargeguard/internal/domain"
	"chargeguard/internal/scheduler"
	"chargeguard/internal/service"
	"chargeguard/internal/sla"
	"chargeguard/internal/storage"
)

func setupTestServer(t *testing.T) (*Server, storage.Store, *domain.FakeClock, context.Context, func()) {
	t.Helper()
	dir := t.TempDir()
	clock := &domain.FakeClock{Current: time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)}
	ctx := context.Background()
	cfg := config.Default()
	cfg.DataDir = dir
	cfg.WriteTimeout = 5 * time.Second
	logger := audit.NewLogger("error")
	store, err := storage.NewStore(ctx, dir, clock)
	require.NoError(t, err)
	bus := service.NewEventBus(store.EventRepo())
	handSvc := service.NewHandoverService(store, clock, bus)
	dispSvc := service.NewDispositionService(store, clock, bus)
	batchSvc := service.NewBatchService(store, clock, bus)
	subSvc := service.NewSubscriptionService(store, clock, bus)
	ledgerSvc := service.NewLedgerService(store, clock)
	importSvc := service.NewImportExportService(handSvc, clock)
	overdueSvc := service.NewOverdueService(store, clock, cfg.SignoffTimeout()).WithSLA(sla.NewRuleSet(cfg.SignoffTimeoutHours))
	maintSvc := service.NewMaintenanceService(store, clock)
	sched := scheduler.New(logger, store, clock)
	sched.AddTask(scheduler.TimeoutMonitorTask(store, clock, cfg.SignoffTimeout()))
	sched.Start(ctx)
	srv := NewServer(&cfg, logger, store, handSvc, dispSvc, batchSvc,
		subSvc, ledgerSvc, importSvc, overdueSvc, maintSvc, sched)
	return srv, store, clock, ctx, func() {
		sched.Stop()
		store.Close()
	}
}

func makeRequest(t *testing.T, method, path string, body string) *http.Request {
	t.Helper()
	var bodyReader *string
	if body != "" {
		bodyReader = &body
	}
	var req *http.Request
	var err error
	if bodyReader != nil {
		req, err = http.NewRequest(method, path, stringReader(*bodyReader))
	} else {
		req, err = http.NewRequest(method, path, nil)
	}
	require.NoError(t, err)
	return req
}

func executeRequest(t *testing.T, srv *Server, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, req)
	return rr
}
