package httpd

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
	"visitor-parking/internal/model"
	"visitor-parking/internal/service"
	"visitor-parking/internal/store"
)

// mustAuthHTTP creates an authorization with the given plate and window,
// returning its id. start/end are offsets from baseTime.
func mustAuthHTTP(t *testing.T, h http.Handler, plate string, startOff, dur time.Duration) string {
	t.Helper()
	area := mustAreaHTTP(t, h, 5)
	res := mustResidentHTTP(t, h, "1栋")
	start := baseTime().Add(startOff)
	body := map[string]interface{}{
		"resident_id": res, "parking_area_id": area, "plate": plate,
		"start_time": start.Format(time.RFC3339), "end_time": start.Add(dur).Format(time.RFC3339),
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
func mustCreateExtension(t *testing.T, h http.Handler, authID, newEnd, reason, applicant string) string {
	t.Helper()
	body := map[string]interface{}{
		"authorization_id": authID,
		"new_end_time":     newEnd,
		"reason":           reason,
		"applicant":        applicant,
	}
	resp, data := doJSON(t, h, http.MethodPost, "/api/v1/extension-applications", body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create extension status=%d body=%s", resp.StatusCode, string(data))
	}
	var env struct {
		Data *model.ExtensionApplication `json:"data"`
	}
	if err := json.Unmarshal(data, &env); err != nil || env.Data == nil {
		t.Fatalf("create extension body: %s", string(data))
	}
	return env.Data.ID
}
func TestCreateExtension_HappyPath(t *testing.T) {
	h, _ := newServer(t, 0)
	authID := seed(t, h)                                         // start=10:00 end=15:00
	newEnd := baseTime().Add(7 * time.Hour).Format(time.RFC3339) // 17:00, total 7h <= 7d
	appID := mustCreateExtension(t, h, authID, newEnd, "访客需多停留", "物业小王")
	resp, data := doJSON(t, h, http.MethodGet, "/api/v1/extension-applications/"+appID, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get status=%d body=%s", resp.StatusCode, string(data))
	}
	var env struct {
		Data *model.ExtensionApplication `json:"data"`
	}
	json.Unmarshal(data, &env)
	if env.Data.Status != model.ExtStatusPending {
		t.Fatalf("status=%s want pending", env.Data.Status)
	}
	if !env.Data.NewEndTime.Equal(baseTime().Add(7 * time.Hour)) {
		t.Fatalf("new_end_time=%v want 17:00", env.Data.NewEndTime)
	}
	if env.Data.OriginalEndTime.IsZero() || env.Data.Plate != "京A12345" {
		t.Fatalf("original_end/plate not populated: %+v", env.Data)
	}
}
func TestCreateExtension_NewEndNotAfterOriginal(t *testing.T) {
	h, _ := newServer(t, 0)
	authID := seed(t, h) // end=15:00
	// new_end equal to original end -> field error
	body := map[string]interface{}{
		"authorization_id": authID,
		"new_end_time":     baseTime().Add(5 * time.Hour).Format(time.RFC3339),
		"reason":           "x", "applicant": "a",
	}
	resp, data := doJSON(t, h, http.MethodPost, "/api/v1/extension-applications", body)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 body=%s", resp.StatusCode, string(data))
	}
	var env envelope
	json.Unmarshal(data, &env)
	if env.Error == nil || len(env.Error.Fields) == 0 || env.Error.Fields[0].Field != "new_end_time" {
		t.Fatalf("expected new_end_time field error, got %s", string(data))
	}
}
func TestCreateExtension_ExceedsSevenDays(t *testing.T) {
	h, _ := newServer(t, 0)
	authID := seed(t, h) // start=10:00 end=15:00
	// new_end = start + 7d + 1s -> total > 7d
	newEnd := baseTime().Add(7*24*time.Hour + time.Second).Format(time.RFC3339)
	body := map[string]interface{}{
		"authorization_id": authID,
		"new_end_time":     newEnd,
		"reason":           "x", "applicant": "a",
	}
	resp, data := doJSON(t, h, http.MethodPost, "/api/v1/extension-applications", body)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d want 400 body=%s", resp.StatusCode, string(data))
	}
}
func TestCreateExtension_CompletedAuthRejected(t *testing.T) {
	h, _ := newServer(t, 0)
	authID := seed(t, h)
	doJSON(t, h, http.MethodPost, "/api/v1/authorizations/"+authID+"/entry", nil)
	doJSON(t, h, http.MethodPost, "/api/v1/authorizations/"+authID+"/exit",
		map[string]interface{}{"operator": "g", "note": "done"}) // -> completed
	newEnd := baseTime().Add(10 * time.Hour).Format(time.RFC3339)
	resp, _ := doJSON(t, h, http.MethodPost, "/api/v1/extension-applications", map[string]interface{}{
		"authorization_id": authID, "new_end_time": newEnd, "reason": "x", "applicant": "a",
	})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("completed auth extend status=%d want 409", resp.StatusCode)
	}
}
func TestCreateExtension_CancelledAuthRejected(t *testing.T) {
	h, _ := newServer(t, 0)
	authID := seed(t, h)
	doJSON(t, h, http.MethodPost, "/api/v1/authorizations/"+authID+"/revoke",
		map[string]interface{}{"operator": "mgr", "reason": "cancel"}) // -> cancelled
	newEnd := baseTime().Add(10 * time.Hour).Format(time.RFC3339)
	resp, _ := doJSON(t, h, http.MethodPost, "/api/v1/extension-applications", map[string]interface{}{
		"authorization_id": authID, "new_end_time": newEnd, "reason": "x", "applicant": "a",
	})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("cancelled auth extend status=%d want 409", resp.StatusCode)
	}
}

func TestCreateExtension_ExpiredActiveAuthRejected(t *testing.T) {
	now := baseTime()
	st := store.NewMemory()
	svc := service.NewWithClock(st, func() time.Time { return now })
	h := NewHandler(svc).Routes()
	authID := seed(t, h)

	resp, data := doJSON(t, h, http.MethodPost, "/api/v1/authorizations/"+authID+"/entry", nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("entry status=%d body=%s", resp.StatusCode, string(data))
	}
	now = baseTime().Add(6 * time.Hour)
	resp, data = doJSON(t, h, http.MethodPost, "/api/v1/extension-applications", map[string]interface{}{
		"authorization_id": authID,
		"new_end_time":     baseTime().Add(7 * time.Hour).Format(time.RFC3339),
		"reason":           "visitor needs more time",
		"applicant":        "manager",
	})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expired active auth extension status=%d want 409 body=%s", resp.StatusCode, string(data))
	}
}

func TestCreateExtension_DuplicatePendingConflict(t *testing.T) {
	h, _ := newServer(t, 0)
	authID := seed(t, h)
	mustCreateExtension(t, h, authID, baseTime().Add(7*time.Hour).Format(time.RFC3339), "r1", "a1")
	// a second pending application for the same authorization -> conflict
	resp, _ := doJSON(t, h, http.MethodPost, "/api/v1/extension-applications", map[string]interface{}{
		"authorization_id": authID, "new_end_time": baseTime().Add(8 * time.Hour).Format(time.RFC3339),
		"reason": "r2", "applicant": "a2",
	})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("duplicate pending status=%d want 409", resp.StatusCode)
	}
}
func TestApproveExtension_UpdatesAuthAndAudits(t *testing.T) {
	h, _ := newServer(t, 0)
	authID := seed(t, h) // end=15:00
	appID := mustCreateExtension(t, h, authID, baseTime().Add(7*time.Hour).Format(time.RFC3339), "r", "a")
	resp, data := doJSON(t, h, http.MethodPost, "/api/v1/extension-applications/"+appID+"/approve",
		map[string]interface{}{"approver": "mgr李", "note": "同意延期"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("approve status=%d body=%s", resp.StatusCode, string(data))
	}
	var env struct {
		Data struct {
			Application   *model.ExtensionApplication `json:"application"`
			Authorization *model.Authorization        `json:"authorization"`
		} `json:"data"`
	}
	json.Unmarshal(data, &env)
	if env.Data.Application.Status != model.ExtStatusApproved {
		t.Fatalf("app status=%s want approved", env.Data.Application.Status)
	}
	if env.Data.Application.DecidedBy != "mgr李" || env.Data.Application.DecidedAt == nil {
		t.Fatalf("approver/decided_at not set: %+v", env.Data.Application)
	}
	// authorization end_time extended to the new end
	if !env.Data.Authorization.EndTime.Equal(baseTime().Add(7 * time.Hour)) {
		t.Fatalf("auth end_time=%v want 17:00", env.Data.Authorization.EndTime)
	}
	// re-fetch the authorization to confirm persistence
	_, data = doJSON(t, h, http.MethodGet, "/api/v1/authorizations/"+authID, nil)
	var authEnv struct {
		Data *model.Authorization `json:"data"`
	}
	json.Unmarshal(data, &authEnv)
	if !authEnv.Data.EndTime.Equal(baseTime().Add(7 * time.Hour)) {
		t.Fatalf("persisted auth end_time=%v want 17:00", authEnv.Data.EndTime)
	}
	// audit log written within the transaction
	_, data = doJSON(t, h, http.MethodGet, "/api/v1/audit-logs?entity_type=extension_application", nil)
	var logEnv struct {
		Data struct {
			Items []*model.AuditLog `json:"items"`
		} `json:"data"`
	}
	json.Unmarshal(data, &logEnv)
	var found bool
	for _, l := range logEnv.Data.Items {
		if l.Action == "extension.approve" && l.EntityID == appID && l.Operator == "mgr李" {
			found = true
		}
	}
	if !found {
		t.Fatalf("approval audit log not found: %s", string(data))
	}
}
func TestApproveExtension_OverlapConflict(t *testing.T) {
	h, _ := newServer(t, 0)
	// auth A: 10:00-15:00, plate 京A12345
	authA := seed(t, h)
	// auth B: same plate, 16:00-20:00 (non-overlapping with A originally)
	mustAuthHTTP(t, h, "京A12345", 6*time.Hour, 4*time.Hour)
	// extend A to 17:00 -> new interval [10:00,17:00] overlaps B [16:00,20:00]
	appID := mustCreateExtension(t, h, authA, baseTime().Add(7*time.Hour).Format(time.RFC3339), "r", "a")
	resp, _ := doJSON(t, h, http.MethodPost, "/api/v1/extension-applications/"+appID+"/approve",
		map[string]interface{}{"approver": "mgr", "note": ""})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("overlap approve status=%d want 409", resp.StatusCode)
	}
	// auth A end_time must remain unchanged
	_, data := doJSON(t, h, http.MethodGet, "/api/v1/authorizations/"+authA, nil)
	var env struct {
		Data *model.Authorization `json:"data"`
	}
	json.Unmarshal(data, &env)
	if !env.Data.EndTime.Equal(baseTime().Add(5 * time.Hour)) {
		t.Fatalf("auth end_time changed after failed approve: %v", env.Data.EndTime)
	}
}
func TestRejectExtension(t *testing.T) {
	h, _ := newServer(t, 0)
	authID := seed(t, h)
	appID := mustCreateExtension(t, h, authID, baseTime().Add(7*time.Hour).Format(time.RFC3339), "r", "a")
	resp, data := doJSON(t, h, http.MethodPost, "/api/v1/extension-applications/"+appID+"/reject",
		map[string]interface{}{"approver": "mgr", "reason": "理由不充分"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reject status=%d body=%s", resp.StatusCode, string(data))
	}
	var env struct {
		Data *model.ExtensionApplication `json:"data"`
	}
	json.Unmarshal(data, &env)
	if env.Data.Status != model.ExtStatusRejected || env.Data.DecisionNote != "理由不充分" {
		t.Fatalf("reject body: %+v", env.Data)
	}
	// auth end_time unchanged
	_, d := doJSON(t, h, http.MethodGet, "/api/v1/authorizations/"+authID, nil)
	var authEnv struct {
		Data *model.Authorization `json:"data"`
	}
	json.Unmarshal(d, &authEnv)
	if !authEnv.Data.EndTime.Equal(baseTime().Add(5 * time.Hour)) {
		t.Fatalf("auth end_time changed after reject: %v", authEnv.Data.EndTime)
	}
	// cannot approve a rejected application
	resp, _ = doJSON(t, h, http.MethodPost, "/api/v1/extension-applications/"+appID+"/approve",
		map[string]interface{}{"approver": "mgr"})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("approve rejected app status=%d want 409", resp.StatusCode)
	}
}
func TestRevokeExtension(t *testing.T) {
	h, _ := newServer(t, 0)
	authID := seed(t, h)
	appID := mustCreateExtension(t, h, authID, baseTime().Add(7*time.Hour).Format(time.RFC3339), "r", "a")
	resp, data := doJSON(t, h, http.MethodPost, "/api/v1/extension-applications/"+appID+"/revoke",
		map[string]interface{}{"operator": "小王", "reason": "申请人撤回"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("revoke status=%d body=%s", resp.StatusCode, string(data))
	}
	var env struct {
		Data *model.ExtensionApplication `json:"data"`
	}
	json.Unmarshal(data, &env)
	if env.Data.Status != model.ExtStatusRevoked {
		t.Fatalf("revoke status=%s want revoked", env.Data.Status)
	}
	// after revocation, a new pending application may be created for the same auth
	resp, _ = doJSON(t, h, http.MethodPost, "/api/v1/extension-applications", map[string]interface{}{
		"authorization_id": authID, "new_end_time": baseTime().Add(8 * time.Hour).Format(time.RFC3339),
		"reason": "r2", "applicant": "a2",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("re-create after revoke status=%d want 201", resp.StatusCode)
	}
}
func TestListExtensionApplications_FilterAndPage(t *testing.T) {
	h, _ := newServer(t, 0)
	authA := seed(t, h) // plate 京A12345
	authB := mustAuthHTTP(t, h, "京B22222", 0, 5*time.Hour)
	mustCreateExtension(t, h, authA, baseTime().Add(7*time.Hour).Format(time.RFC3339), "r", "小王")
	mustCreateExtension(t, h, authB, baseTime().Add(7*time.Hour).Format(time.RFC3339), "r", "小李")
	// filter by plate
	resp, data := doJSON(t, h, http.MethodGet, "/api/v1/extension-applications?plate=京A12345", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var env struct {
		Data struct {
			Items []*model.ExtensionApplication `json:"items"`
			Total int                           `json:"total"`
		} `json:"data"`
	}
	json.Unmarshal(data, &env)
	if env.Data.Total != 1 || env.Data.Items[0].Plate != "京A12345" {
		t.Fatalf("plate filter total=%d item0=%+v", env.Data.Total, env.Data.Items[0])
	}
	// filter by authorization_id
	resp, data = doJSON(t, h, http.MethodGet, "/api/v1/extension-applications?authorization_id="+authB, nil)
	json.Unmarshal(data, &env)
	if env.Data.Total != 1 || env.Data.Items[0].AuthorizationID != authB {
		t.Fatalf("auth filter total=%d", env.Data.Total)
	}
	// list all (2) and check status filter
	resp, data = doJSON(t, h, http.MethodGet, "/api/v1/extension-applications?status=pending", nil)
	json.Unmarshal(data, &env)
	if env.Data.Total != 2 {
		t.Fatalf("pending total=%d want 2", env.Data.Total)
	}
}
func TestExtensionNotFound(t *testing.T) {
	h, _ := newServer(t, 0)
	resp, _ := doJSON(t, h, http.MethodGet, "/api/v1/extension-applications/nope", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d want 404", resp.StatusCode)
	}
}
