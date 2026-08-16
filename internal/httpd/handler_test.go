package httpd
import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"visitor-parking/internal/model"
	"visitor-parking/internal/service"
	"visitor-parking/internal/store"
)
func baseTime() time.Time {
	return time.Date(2026, 8, 16, 10, 0, 0, 0, time.UTC)
}
func newServer(t *testing.T, advance time.Duration) (http.Handler, *service.Service) {
	t.Helper()
	st := store.NewMemory()
	svc := service.NewWithClock(st, func() time.Time { return baseTime().Add(advance) })
	return NewHandler(svc).Routes(), svc
}
func doJSON(t *testing.T, h http.Handler, method, path string, body interface{}) (*http.Response, []byte) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	resp := w.Result()
	data, _ := io.ReadAll(resp.Body)
	return resp, data
}
func seed(t *testing.T, h http.Handler) string {
	t.Helper()
	area := mustAreaHTTP(t, h, 5)
	res := mustResidentHTTP(t, h, "1栋")
	start := baseTime()
	end := start.Add(5 * time.Hour)
	body := map[string]interface{}{
		"resident_id": res, "parking_area_id": area, "plate": "京A12345",
		"start_time": start.Format(time.RFC3339), "end_time": end.Format(time.RFC3339),
		"created_by": "staff1",
	}
	_, data := doJSON(t, h, http.MethodPost, "/api/v1/authorizations", body)
	var env struct {
		Data *model.Authorization `json:"data"`
	}
	if err := json.Unmarshal(data, &env); err != nil || env.Data == nil {
		t.Fatalf("create auth: %s", string(data))
	}
	return env.Data.ID
}
func mustAreaHTTP(t *testing.T, h http.Handler, cap int) string {
	t.Helper()
	_, data := doJSON(t, h, http.MethodPost, "/api/v1/areas",
		map[string]interface{}{"name": "A区", "code": "A", "capacity": cap})
	var env struct {
		Data *model.ParkingArea `json:"data"`
	}
	if err := json.Unmarshal(data, &env); err != nil || env.Data == nil {
		t.Fatalf("create area: %s", string(data))
	}
	return env.Data.ID
}
func mustResidentHTTP(t *testing.T, h http.Handler, building string) string {
	t.Helper()
	_, data := doJSON(t, h, http.MethodPost, "/api/v1/residents",
		map[string]interface{}{"name": "张三", "phone": "13800000000", "building": building, "unit": "1", "room": "101"})
	var env struct {
		Data *model.Resident `json:"data"`
	}
	if err := json.Unmarshal(data, &env); err != nil || env.Data == nil {
		t.Fatalf("create resident: %s", string(data))
	}
	return env.Data.ID
}
func TestHealthz(t *testing.T) {
	h, _ := newServer(t, 0)
	resp, _ := doJSON(t, h, http.MethodGet, "/healthz", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}
func TestCreateResident_ValidationErrorShape(t *testing.T) {
	h, _ := newServer(t, 0)
	resp, data := doJSON(t, h, http.MethodPost, "/api/v1/residents", map[string]interface{}{"phone": "1", "building": "1"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Error == nil || len(env.Error.Fields) == 0 {
		t.Fatalf("expected field errors, got %s", string(data))
	}
	if env.Error.Fields[0].Field != "name" {
		t.Fatalf("expected name field, got %s", env.Error.Fields[0].Field)
	}
}
func TestFullReportToExitFlow(t *testing.T) {
	h, _ := newServer(t, 0)
	authID := seed(t, h)
	resp, data := doJSON(t, h, http.MethodPost, "/api/v1/authorizations/"+authID+"/entry", nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("entry status=%d body=%s", resp.StatusCode, string(data))
	}
	var entryEnv struct {
		Data *model.EntryExitRecord `json:"data"`
	}
	if err := json.Unmarshal(data, &entryEnv); err != nil || entryEnv.Data == nil {
		t.Fatalf("entry body: %s", string(data))
	}
	if entryEnv.Data.Status != model.RecordStatusEntered {
		t.Fatalf("entry status=%s", entryEnv.Data.Status)
	}
	resp, _ = doJSON(t, h, http.MethodPost, "/api/v1/authorizations/"+authID+"/entry", nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate entry status=%d", resp.StatusCode)
	}
	resp, data = doJSON(t, h, http.MethodPost, "/api/v1/authorizations/"+authID+"/exit",
		map[string]interface{}{"operator": "guard1", "note": "normal exit"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("exit status=%d body=%s", resp.StatusCode, string(data))
	}
	var exitEnv struct {
		Data *model.EntryExitRecord `json:"data"`
	}
	json.Unmarshal(data, &exitEnv)
	if exitEnv.Data.Status != model.RecordStatusExited || exitEnv.Data.ExitOperator != "guard1" {
		t.Fatalf("exit body: %s", string(data))
	}
	resp, _ = doJSON(t, h, http.MethodPost, "/api/v1/authorizations/"+authID+"/exit",
		map[string]interface{}{"operator": "guard1"})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("re-exit status=%d", resp.StatusCode)
	}
}
func TestRevokeActiveRejected(t *testing.T) {
	h, _ := newServer(t, 0)
	authID := seed(t, h)
	doJSON(t, h, http.MethodPost, "/api/v1/authorizations/"+authID+"/entry", nil)
	resp, _ := doJSON(t, h, http.MethodPost, "/api/v1/authorizations/"+authID+"/revoke",
		map[string]interface{}{"operator": "mgr", "reason": "test"})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("revoke active status=%d (want conflict)", resp.StatusCode)
	}
}
func TestRevokePendingOK(t *testing.T) {
	h, _ := newServer(t, 0)
	authID := seed(t, h)
	resp, data := doJSON(t, h, http.MethodPost, "/api/v1/authorizations/"+authID+"/revoke",
		map[string]interface{}{"operator": "mgr", "reason": "resident cancelled"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("revoke status=%d body=%s", resp.StatusCode, string(data))
	}
}
func TestListAuthorizations_FilterAndSort(t *testing.T) {
	h, _ := newServer(t, 0)
	area := mustAreaHTTP(t, h, 5)
	res := mustResidentHTTP(t, h, "1栋")
	start := baseTime()
	end := start.Add(2 * time.Hour)
	doJSON(t, h, http.MethodPost, "/api/v1/authorizations", map[string]interface{}{
		"resident_id": res, "parking_area_id": area, "plate": "京Z11111",
		"start_time": start.Format(time.RFC3339), "end_time": end.Format(time.RFC3339),
	})
	doJSON(t, h, http.MethodPost, "/api/v1/authorizations", map[string]interface{}{
		"resident_id": res, "parking_area_id": area, "plate": "京Z22222",
		"start_time": start.Add(time.Hour).Format(time.RFC3339), "end_time": end.Add(time.Hour).Format(time.RFC3339),
	})
	resp, data := doJSON(t, h, http.MethodGet, "/api/v1/authorizations?plate=京Z22222", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var env struct {
		Data struct {
			Items []*model.Authorization `json:"items"`
			Total int                    `json:"total"`
		} `json:"data"`
	}
	json.Unmarshal(data, &env)
	if env.Data.Total != 1 {
		t.Fatalf("filter total=%d want 1", env.Data.Total)
	}
	resp, data = doJSON(t, h, http.MethodGet, "/api/v1/authorizations", nil)
	json.Unmarshal(data, &env)
	if env.Data.Total != 2 {
		t.Fatalf("total=%d want 2", env.Data.Total)
	}
	if !env.Data.Items[0].StartTime.After(env.Data.Items[1].StartTime) {
		t.Fatalf("expected desc sort by start_time")
	}
}
func TestUpdateResident_ConcurrentModify(t *testing.T) {
	h, _ := newServer(t, 0)
	resID := mustResidentHTTP(t, h, "1栋")
	_, data := doJSON(t, h, http.MethodGet, "/api/v1/residents/"+resID, nil)
	var env struct {
		Data *model.Resident `json:"data"`
	}
	json.Unmarshal(data, &env)
	resp, _ := doJSON(t, h, http.MethodPut, "/api/v1/residents/"+resID, map[string]interface{}{
		"name": "张三改", "phone": "13800000001", "building": "1栋",
		"updated_at": env.Data.UpdatedAt.Add(-time.Second).Format(time.RFC3339Nano),
	})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("stale update status=%d want 409", resp.StatusCode)
	}
	resp, _ = doJSON(t, h, http.MethodPut, "/api/v1/residents/"+resID, map[string]interface{}{
		"name": "张三改", "phone": "13800000001", "building": "1栋",
		"updated_at": env.Data.UpdatedAt.Format(time.RFC3339Nano),
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update status=%d want 200", resp.StatusCode)
	}
}
func TestArchiveResident_SoftDelete(t *testing.T) {
	h, _ := newServer(t, 0)
	resID := mustResidentHTTP(t, h, "1栋")
	resp, _ := doJSON(t, h, http.MethodDelete, "/api/v1/residents/"+resID, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("archive status=%d", resp.StatusCode)
	}
	resp, _ = doJSON(t, h, http.MethodGet, "/api/v1/residents/"+resID, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("get archived status=%d want 404", resp.StatusCode)
	}
}
func TestStatsEndpoints(t *testing.T) {
	h, _ := newServer(t, 0)
	authID := seed(t, h)
	resp, data := doJSON(t, h, http.MethodGet, "/api/v1/stats/current-vehicles", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("current status=%d", resp.StatusCode)
	}
	var env struct {
		Data struct {
			Total int `json:"total"`
		} `json:"data"`
	}
	json.Unmarshal(data, &env)
	if env.Data.Total != 0 {
		t.Fatalf("current before entry=%d want 0", env.Data.Total)
	}
	doJSON(t, h, http.MethodPost, "/api/v1/authorizations/"+authID+"/entry", nil)
	resp, data = doJSON(t, h, http.MethodGet, "/api/v1/stats/occupancy", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("occupancy status=%d", resp.StatusCode)
	}
	var occ struct {
		Data struct {
			Areas []*model.AreaOccupancy `json:"areas"`
		} `json:"data"`
	}
	json.Unmarshal(data, &occ)
	if len(occ.Data.Areas) == 0 || occ.Data.Areas[0].Occupied != 1 {
		t.Fatalf("occupancy after entry: %s", string(data))
	}
	resp, data = doJSON(t, h, http.MethodGet, "/api/v1/stats/today-arrivals", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("today status=%d", resp.StatusCode)
	}
	var arr struct {
		Data struct {
			Total int `json:"total"`
		} `json:"data"`
	}
	json.Unmarshal(data, &arr)
	if arr.Data.Total < 1 {
		t.Fatalf("today-arrivals=%d want >=1", arr.Data.Total)
	}
}
func TestAuditLogsCreated(t *testing.T) {
	h, _ := newServer(t, 0)
	mustResidentHTTP(t, h, "1栋")
	resp, data := doJSON(t, h, http.MethodGet, "/api/v1/audit-logs?entity_type=resident", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("audit status=%d", resp.StatusCode)
	}
	var env struct {
		Data struct {
			Items []*model.AuditLog `json:"items"`
		} `json:"data"`
	}
	json.Unmarshal(data, &env)
	if len(env.Data.Items) == 0 {
		t.Fatalf("expected audit logs, got %s", string(data))
	}
}
func TestNotFound(t *testing.T) {
	h, _ := newServer(t, 0)
	resp, _ := doJSON(t, h, http.MethodGet, "/api/v1/residents/nope", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d want 404", resp.StatusCode)
	}
}
func TestStructuredRequestLogging_NoPanic(t *testing.T) {
	st := store.NewMemory()
	svc := service.NewWithClock(st, func() time.Time { return baseTime() })
	handler := WithLogging(nil, NewHandler(svc).Routes())
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler = WithLogging(logger, NewHandler(svc).Routes())
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("healthz=%d", w.Code)
	}
}
