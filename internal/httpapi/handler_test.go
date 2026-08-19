package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"chargeguard/internal/domain"
)

func TestLoginAndLogoutRevokesSession(t *testing.T) {
	srv, _, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	req := makeRequest(t, "POST", "/api/v1/auth/login", `{"username":"regulator","password":"regulator-demo"}`)
	rr := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", rr.Code, rr.Body.String())
	}
	var session struct{ ID string }
	if err := json.Unmarshal(rr.Body.Bytes(), &session); err != nil || session.ID == "" {
		t.Fatalf("invalid session: %v %s", err, rr.Body.String())
	}
	logout := makeRequest(t, "POST", "/api/v1/auth/logout", "")
	logout.Header.Set("Authorization", "Bearer "+session.ID)
	logoutResult := httptest.NewRecorder()
	srv.Routes().ServeHTTP(logoutResult, logout)
	if logoutResult.Code != http.StatusNoContent {
		t.Fatalf("logout status=%d body=%s", logoutResult.Code, logoutResult.Body.String())
	}
	repeated := makeRequest(t, "POST", "/api/v1/auth/logout", "")
	repeated.Header.Set("Authorization", "Bearer "+session.ID)
	repeatedResult := httptest.NewRecorder()
	srv.Routes().ServeHTTP(repeatedResult, repeated)
	if repeatedResult.Code != http.StatusUnauthorized {
		t.Fatalf("revoked status=%d", repeatedResult.Code)
	}
}

func TestHealthz(t *testing.T) {
	srv, _, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	req := makeRequest(t, "GET", "/healthz", "")
	rr := executeRequest(t, srv, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	var resp map[string]string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, "ok", resp["status"])
}

func TestReadyz(t *testing.T) {
	srv, _, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	req := makeRequest(t, "GET", "/readyz", "")
	rr := executeRequest(t, srv, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, "ready", resp["status"])
}

func TestRegisterHandoverAPI(t *testing.T) {
	srv, _, _, ctx, cleanup := setupTestServer(t)
	defer cleanup()
	body := `{"form_no":"H-API-001","date":"2026-08-19","route_id":"R001","vehicle_no":"V001","outbound_station":"JMS-01","arrival_station":"JMS-02","responsible":"tester","mail_items":[{"mail_no":"M-API-1","sender_name":"S","receiver_name":"R"}]}`
	req := makeRequest(t, "POST", "/api/v1/handovers", body)
	req.Header.Set("Content-Type", "application/json")
	rr := executeRequest(t, srv, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	_ = ctx
	var form map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &form))
	assert.Equal(t, "H-API-001", form["form_no"])
	assert.Equal(t, "draft", form["state"])
}

func TestGetHandoverNotFound(t *testing.T) {
	srv, _, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	req := makeRequest(t, "GET", "/api/v1/handovers/nonexistent", "")
	rr := executeRequest(t, srv, req)
	assert.Equal(t, http.StatusNotFound, rr.Code)
	var errResp ErrorResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &errResp))
	assert.Equal(t, "not_found", errResp.Error.Code)
}

func TestPaginationAPI(t *testing.T) {
	srv, store, clock, ctx, cleanup := setupTestServer(t)
	defer cleanup()
	for i := 0; i < 15; i++ {
		form := &domain.HandoverForm{
			ID: "h-page-" + itoa(i), FormNo: "HP" + itoa(i), Date: "2026-08-19",
			RouteID: "R001", VehicleNo: "V001", State: domain.HandoverStateDraft,
			MailItemCount: 1, Responsible: "test",
			RegisteredAt: clock.Now(), UpdatedAt: clock.Now(),
			Version: 1, ShardID: domain.ShardIDFor("2026-08-19", "R001"), DataVersion: 1,
		}
		require.NoError(t, store.HandoverRepo().Save(ctx, form))
		clock.Advance(1e9)
	}
	req := makeRequest(t, "GET", "/api/v1/handovers?page_size=10&page_offset=0", "")
	rr := executeRequest(t, srv, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	var resp PaginatedResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, 15, resp.Total)
	assert.Equal(t, 10, resp.PageSize)
	assert.True(t, resp.HasNext)
}

func TestImportHandoversCSV(t *testing.T) {
	srv, _, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	csvBody := "form_no,date,route_id,vehicle_no,outbound_station,arrival_station,responsible,mail_no,sender_name,sender_addr,receiver_name,receiver_addr\n" +
		"H-IMP-1,2026-08-19,R001,V001,JMS-01,JMS-02,tester,M-IMP-1,S1,addr1,R1,addr2\n" +
		"H-IMP-2,2026-08-19,R001,V001,JMS-01,JMS-02,tester,M-IMP-2,S2,addr1,R2,addr2\n" +
		",2026-08-19,R001,V001,JMS-01,JMS-02,tester,M-IMP-3,S3,addr1,R3,addr2\n"
	req := makeRequest(t, "POST", "/api/v1/imports/handovers", csvBody)
	req.Header.Set("Content-Type", "text/csv")
	rr := executeRequest(t, srv, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	var result map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &result))
	assert.Equal(t, float64(3), result["total_rows"])
	assert.Equal(t, float64(2), result["succeeded"])
	assert.Equal(t, float64(1), result["failed"])
}

func TestExportLedgerAPI(t *testing.T) {
	srv, store, clock, ctx, cleanup := setupTestServer(t)
	defer cleanup()
	entry := &domain.LedgerEntry{
		ID: "le-1", Date: "2026-08-19", RouteID: "R001", VolumeNo: "2026-08-19_R001",
		FormNo: "H-EXP-1", EntryType: domain.LedgerEntryTypeRegistration,
		MailNo: "M-EXP-1", Responsible: "tester", Description: "registered",
		CreatedAt: clock.Now(), ShardID: domain.ShardIDFor("2026-08-19", "R001"), DataVersion: 1,
	}
	require.NoError(t, store.LedgerRepo().Save(ctx, entry))
	req := makeRequest(t, "GET", "/api/v1/ledgers/export?date=2026-08-19&route_id=R001", "")
	rr := executeRequest(t, srv, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "H-EXP-1")
	assert.Contains(t, rr.Header().Get("Content-Type"), "text/csv")
}

func TestSubmitDispositionAPI(t *testing.T) {
	srv, store, clock, ctx, cleanup := setupTestServer(t)
	defer cleanup()
	mail := &domain.MailItem{
		ID: "mail-api-1", MailNo: "MN-API-1", RouteID: "R001", VehicleNo: "V001",
		State: domain.MailStateInTransit, OriginStation: "JMS-01", DestStation: "JMS-02",
		SenderName: "S", ReceiverName: "R", Responsible: "test",
		RegisteredAt: clock.Now(), Version: 1,
		ShardID: domain.ShardIDFor("2026-08-19", "R001"), DataVersion: 1,
	}
	require.NoError(t, store.MailItemRepo().Save(ctx, mail))
	body := `{"mail_id":"mail-api-1","type":"intercept","submitted_by":"station-op"}`
	req := makeRequest(t, "POST", "/api/v1/dispositions", body)
	req.Header.Set("Content-Type", "application/json")
	rr := executeRequest(t, srv, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	var disp map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &disp))
	assert.Equal(t, "pending", disp["state"])
}

func TestOverduesAPI(t *testing.T) {
	srv, store, clock, ctx, cleanup := setupTestServer(t)
	defer cleanup()
	mail := &domain.MailItem{
		ID: "mail-od-1", MailNo: "MN-OD-1", RouteID: "R001", VehicleNo: "V001",
		State: domain.MailStateInTransit, OriginStation: "JMS-01", DestStation: "JMS-02",
		SenderName: "S", ReceiverName: "R", Responsible: "test",
		RegisteredAt: clock.Now().Add(-72 * 3600e9), Version: 1,
		ShardID: domain.ShardIDFor("2026-08-19", "R001"), DataVersion: 1,
	}
	require.NoError(t, store.MailItemRepo().Save(ctx, mail))
	req := makeRequest(t, "GET", "/api/v1/overdues", "")
	rr := executeRequest(t, srv, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	var report map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &report))
	assert.NotNil(t, report["overdue_mails"])
}

func TestVerifyDataAPI(t *testing.T) {
	srv, _, _, _, cleanup := setupTestServer(t)
	defer cleanup()
	req := makeRequest(t, "POST", "/api/v1/maintenance/verify", "")
	rr := executeRequest(t, srv, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	var report map[string]any
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &report))
	assert.Contains(t, report, "total_shards")
}
