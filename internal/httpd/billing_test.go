package httpd

import (
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"
	"visitor-parking/internal/model"
	"visitor-parking/internal/service"
	"visitor-parking/internal/store"
)

// newClockServer returns a server backed by a mutable clock. The returned
// *time.Time lets the test advance time between entry and exit so a real,
// non-zero parking duration is billed.
func newClockServer(t *testing.T) (http.Handler, *service.Service, *time.Time) {
	t.Helper()
	clk := baseTime()
	st := store.NewMemory()
	svc := service.NewWithClock(st, func() time.Time { return clk })
	return NewHandler(svc).Routes(), svc, &clk
}

func advance(clk *time.Time, d time.Duration) { *clk = clk.Add(d) }

func mustBillingRuleHTTP(t *testing.T, h http.Handler, areaID string, free int, rate, cap int64) string {
	t.Helper()
	_, data := doJSON(t, h, http.MethodPost, "/api/v1/billing-rules", map[string]interface{}{
		"parking_area_id":   areaID,
		"free_minutes":      free,
		"hourly_rate_cents": rate,
		"daily_cap_cents":   cap,
	})
	var env struct {
		Data *model.BillingRule `json:"data"`
	}
	if err := json.Unmarshal(data, &env); err != nil || env.Data == nil {
		t.Fatalf("create billing rule: %s", string(data))
	}
	return env.Data.ID
}

// fullExitAndFee seeds an area + billing rule + resident + auth, performs entry,
// advances the clock, then exits. Returns the created fee (or fails) and the
// record id.
func fullExitAndFee(t *testing.T, h http.Handler, clk *time.Time, free int, rate, cap int64, dwell time.Duration) (*model.Fee, string) {
	t.Helper()
	area := mustAreaHTTP(t, h, 5)
	mustBillingRuleHTTP(t, h, area, free, rate, cap)
	res := mustResidentHTTP(t, h, "1栋")
	start := *clk
	end := start.Add(48 * time.Hour)
	_, data := doJSON(t, h, http.MethodPost, "/api/v1/authorizations", map[string]interface{}{
		"resident_id": res, "parking_area_id": area, "plate": "京A12345",
		"start_time": start.Format(time.RFC3339), "end_time": end.Format(time.RFC3339),
		"created_by": "staff1",
	})
	var authEnv struct {
		Data *model.Authorization `json:"data"`
	}
	if err := json.Unmarshal(data, &authEnv); err != nil || authEnv.Data == nil {
		t.Fatalf("create auth: %s", string(data))
	}
	if _, d := doJSON(t, h, http.MethodPost, "/api/v1/authorizations/"+authEnv.Data.ID+"/entry", nil); d != nil {
	}
	advance(clk, dwell)
	_, edata := doJSON(t, h, http.MethodPost, "/api/v1/authorizations/"+authEnv.Data.ID+"/exit",
		map[string]interface{}{"operator": "guard1", "note": "normal"})
	var exitEnv struct {
		Data *model.EntryExitRecord `json:"data"`
	}
	if err := json.Unmarshal(edata, &exitEnv); err != nil || exitEnv.Data == nil {
		t.Fatalf("exit: %s", string(edata))
	}
	// fetch the fee created by exit
	_, fdata := doJSON(t, h, http.MethodGet, "/api/v1/fees?record_id="+exitEnv.Data.ID, nil)
	var feeEnv struct {
		Data struct {
			Items []*model.Fee `json:"items"`
			Total int          `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(fdata, &feeEnv); err != nil {
		t.Fatalf("list fees: %s", string(fdata))
	}
	if feeEnv.Data.Total != 1 || len(feeEnv.Data.Items) != 1 {
		t.Fatalf("expected 1 fee, got %s", string(fdata))
	}
	return feeEnv.Data.Items[0], exitEnv.Data.ID
}

func TestBilling_ExitGeneratesFee(t *testing.T) {
	h, _, clk := newClockServer(t)
	fee, _ := fullExitAndFee(t, h, clk, 30, 500, 4000, 2*time.Hour)
	// 2h dwell, 30min free -> 90 billable -> ceil 2h -> 2*500 = 1000
	if fee.AmountCents != 1000 {
		t.Fatalf("amount=%d want 1000", fee.AmountCents)
	}
	if fee.Status != model.FeeStatusUnsettled {
		t.Fatalf("status=%s want unsettled", fee.Status)
	}
	if fee.ChargedMinutes != 90 {
		t.Fatalf("charged=%d want 90", fee.ChargedMinutes)
	}
	if fee.DurationMinutes != 120 {
		t.Fatalf("duration=%d want 120", fee.DurationMinutes)
	}
}

func TestBilling_FreePeriodZeroFee(t *testing.T) {
	h, _, clk := newClockServer(t)
	fee, _ := fullExitAndFee(t, h, clk, 60, 500, 4000, 20*time.Minute)
	// 20 min within 60 free -> 0
	if fee.AmountCents != 0 {
		t.Fatalf("free stay amount=%d want 0", fee.AmountCents)
	}
	if fee.ChargedMinutes != 0 {
		t.Fatalf("charged=%d want 0", fee.ChargedMinutes)
	}
}

func TestBilling_DailyCapApplied(t *testing.T) {
	h, _, clk := newClockServer(t)
	fee, _ := fullExitAndFee(t, h, clk, 0, 500, 2000, 6*time.Hour)
	// 6h * 500 = 3000, cap 2000
	if fee.AmountCents != 2000 {
		t.Fatalf("amount=%d want 2000 (capped)", fee.AmountCents)
	}
}

func TestBilling_CrossDaySeparateCap(t *testing.T) {
	h, _, clk := newClockServer(t)
	// set clock to 18:00 day1, dwell 12h -> exit 06:00 day2
	*clk = time.Date(2026, 8, 16, 18, 0, 0, 0, time.UTC)
	fee, _ := fullExitAndFee(t, h, clk, 0, 500, 2000, 12*time.Hour)
	// day1 6h -> 3000 cap 2000; day2 6h -> 3000 cap 2000; total 4000
	if fee.AmountCents != 4000 {
		t.Fatalf("amount=%d want 4000 (two capped days)", fee.AmountCents)
	}
}

func TestBilling_SettleCashThenResettleConflict(t *testing.T) {
	h, _, clk := newClockServer(t)
	fee, _ := fullExitAndFee(t, h, clk, 30, 500, 4000, 2*time.Hour)
	resp, data := doJSON(t, h, http.MethodPost, "/api/v1/fees/"+fee.ID+"/settle",
		map[string]interface{}{"method": "cash", "operator": "cashier"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("settle status=%d body=%s", resp.StatusCode, string(data))
	}
	var env struct {
		Data *model.Fee `json:"data"`
	}
	json.Unmarshal(data, &env)
	if env.Data.Status != model.FeeStatusSettled || env.Data.SettleMethod != "cash" {
		t.Fatalf("settle body: %s", string(data))
	}
	// re-settle -> 409
	resp, _ = doJSON(t, h, http.MethodPost, "/api/v1/fees/"+fee.ID+"/settle",
		map[string]interface{}{"method": "online"})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("re-settle status=%d want 409", resp.StatusCode)
	}
}

func TestBilling_WaiverRequiresReason(t *testing.T) {
	h, _, clk := newClockServer(t)
	fee, _ := fullExitAndFee(t, h, clk, 30, 500, 4000, 2*time.Hour)
	resp, data := doJSON(t, h, http.MethodPost, "/api/v1/fees/"+fee.ID+"/settle",
		map[string]interface{}{"method": "waiver"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("waiver no reason status=%d want 400 body=%s", resp.StatusCode, string(data))
	}
	var env envelope
	json.Unmarshal(data, &env)
	if env.Error == nil || len(env.Error.Fields) == 0 || env.Error.Fields[0].Field != "reason" {
		t.Fatalf("expected reason field error, got %s", string(data))
	}
	// with reason -> ok
	resp, data = doJSON(t, h, http.MethodPost, "/api/v1/fees/"+fee.ID+"/settle",
		map[string]interface{}{"method": "waiver", "reason": "物业赠送", "operator": "mgr"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("waiver with reason status=%d body=%s", resp.StatusCode, string(data))
	}
}

func TestBilling_InvalidSettleMethod(t *testing.T) {
	h, _, clk := newClockServer(t)
	fee, _ := fullExitAndFee(t, h, clk, 30, 500, 4000, 2*time.Hour)
	resp, data := doJSON(t, h, http.MethodPost, "/api/v1/fees/"+fee.ID+"/settle",
		map[string]interface{}{"method": "bitcoin"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid method status=%d want 400 body=%s", resp.StatusCode, string(data))
	}
	var env envelope
	json.Unmarshal(data, &env)
	if env.Error == nil || len(env.Error.Fields) == 0 || env.Error.Fields[0].Field != "method" {
		t.Fatalf("expected method field error, got %s", string(data))
	}
}

func TestBilling_UnsettledFeeQuery(t *testing.T) {
	h, _, clk := newClockServer(t)
	_, _ = fullExitAndFee(t, h, clk, 30, 500, 4000, 2*time.Hour)
	advance(clk, 0)
	// second fee
	_, _ = fullExitAndFee(t, h, clk, 30, 500, 4000, 1*time.Hour)
	resp, data := doJSON(t, h, http.MethodGet, "/api/v1/fees?status=unsettled", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	var env struct {
		Data struct {
			Items []*model.Fee `json:"items"`
			Total int          `json:"total"`
		} `json:"data"`
	}
	json.Unmarshal(data, &env)
	if env.Data.Total != 2 {
		t.Fatalf("unsettled total=%d want 2", env.Data.Total)
	}
	for _, f := range env.Data.Items {
		if f.Status != model.FeeStatusUnsettled {
			t.Fatalf("expected unsettled, got %s", f.Status)
		}
	}
}

func TestBilling_AreaRevenue(t *testing.T) {
	h, _, clk := newClockServer(t)
	area := mustAreaHTTP(t, h, 5)
	mustBillingRuleHTTP(t, h, area, 30, 500, 4000)
	res := mustResidentHTTP(t, h, "1栋")

	exitAndFee := func(plate string, dwell time.Duration) *model.Fee {
		start := *clk
		end := start.Add(48 * time.Hour)
		_, data := doJSON(t, h, http.MethodPost, "/api/v1/authorizations", map[string]interface{}{
			"resident_id": res, "parking_area_id": area, "plate": plate,
			"start_time": start.Format(time.RFC3339), "end_time": end.Format(time.RFC3339),
		})
		var authEnv struct {
			Data *model.Authorization `json:"data"`
		}
		json.Unmarshal(data, &authEnv)
		doJSON(t, h, http.MethodPost, "/api/v1/authorizations/"+authEnv.Data.ID+"/entry", nil)
		advance(clk, dwell)
		_, edata := doJSON(t, h, http.MethodPost, "/api/v1/authorizations/"+authEnv.Data.ID+"/exit",
			map[string]interface{}{"operator": "g"})
		var exitEnv struct {
			Data *model.EntryExitRecord `json:"data"`
		}
		json.Unmarshal(edata, &exitEnv)
		_, fdata := doJSON(t, h, http.MethodGet, "/api/v1/fees?record_id="+exitEnv.Data.ID, nil)
		var feeEnv struct {
			Data struct {
				Items []*model.Fee `json:"items"`
			} `json:"data"`
		}
		json.Unmarshal(fdata, &feeEnv)
		return feeEnv.Data.Items[0]
	}

	fee1 := exitAndFee("京A11111", 2*time.Hour) // 1000
	fee2 := exitAndFee("京A22222", 1*time.Hour) // 1h -30 free = 30 billable -> 1h -> 500
	// settle fee1 only
	doJSON(t, h, http.MethodPost, "/api/v1/fees/"+fee1.ID+"/settle", map[string]interface{}{"method": "cash"})
	resp, data := doJSON(t, h, http.MethodGet, "/api/v1/stats/area-revenue", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("revenue status=%d", resp.StatusCode)
	}
	var env struct {
		Data struct {
			Areas []*model.AreaRevenue `json:"areas"`
		} `json:"data"`
	}
	json.Unmarshal(data, &env)
	if len(env.Data.Areas) == 0 {
		t.Fatalf("no areas in revenue: %s", string(data))
	}
	var a *model.AreaRevenue
	for _, ar := range env.Data.Areas {
		if ar.AreaID == area {
			a = ar
		}
	}
	if a == nil {
		t.Fatalf("area not in revenue: %s", string(data))
	}
	if a.FeeCount != 2 {
		t.Fatalf("fee count=%d want 2", a.FeeCount)
	}
	if a.SettledCount != 1 || a.UnsettledCount != 1 {
		t.Fatalf("settled/unsettled=%d/%d want 1/1", a.SettledCount, a.UnsettledCount)
	}
	if a.SettledCents != 1000 || a.UnsettledCents != 500 {
		t.Fatalf("settled/unsettled cents=%d/%d want 1000/500", a.SettledCents, a.UnsettledCents)
	}
	if a.TotalCents != 1500 {
		t.Fatalf("total=%d want 1500", a.TotalCents)
	}
	_ = fee2
}

func TestBillingRule_CRUD(t *testing.T) {
	h, _, clk := newClockServer(t)
	area := mustAreaHTTP(t, h, 5)
	// create
	ruleID := mustBillingRuleHTTP(t, h, area, 15, 800, 5000)
	// get
	resp, data := doJSON(t, h, http.MethodGet, "/api/v1/billing-rules/"+ruleID, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get status=%d body=%s", resp.StatusCode, string(data))
	}
	var env struct {
		Data *model.BillingRule `json:"data"`
	}
	json.Unmarshal(data, &env)
	if env.Data.FreeMinutes != 15 || env.Data.HourlyRateCents != 800 || env.Data.DailyCapCents != 5000 {
		t.Fatalf("rule body: %s", string(data))
	}
	if env.Data.Status != model.BillingRuleActive {
		t.Fatalf("status=%s want active", env.Data.Status)
	}
	// list
	resp, data = doJSON(t, h, http.MethodGet, "/api/v1/billing-rules?area_id="+area, nil)
	var listEnv struct {
		Data struct {
			Total int `json:"total"`
		} `json:"data"`
	}
	json.Unmarshal(data, &listEnv)
	if listEnv.Data.Total != 1 {
		t.Fatalf("list total=%d want 1", listEnv.Data.Total)
	}
	// update (optimistic, requires updated_at)
	_, data = doJSON(t, h, http.MethodGet, "/api/v1/billing-rules/"+ruleID, nil)
	json.Unmarshal(data, &env)
	advance(clk, time.Second)
	resp, data = doJSON(t, h, http.MethodPut, "/api/v1/billing-rules/"+ruleID, map[string]interface{}{
		"hourly_rate_cents": 900,
		"updated_at":        env.Data.UpdatedAt.Format(time.RFC3339Nano),
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update status=%d body=%s", resp.StatusCode, string(data))
	}
	json.Unmarshal(data, &env)
	if env.Data.HourlyRateCents != 900 {
		t.Fatalf("updated rate=%d want 900", env.Data.HourlyRateCents)
	}
	// stale update -> 409
	resp, _ = doJSON(t, h, http.MethodPut, "/api/v1/billing-rules/"+ruleID, map[string]interface{}{
		"hourly_rate_cents": 100,
		"updated_at":        env.Data.UpdatedAt.Add(-time.Second).Format(time.RFC3339Nano),
	})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("stale update status=%d want 409", resp.StatusCode)
	}
	// archive
	resp, _ = doJSON(t, h, http.MethodDelete, "/api/v1/billing-rules/"+ruleID, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("archive status=%d want 204", resp.StatusCode)
	}
	// get archived -> 404
	resp, _ = doJSON(t, h, http.MethodGet, "/api/v1/billing-rules/"+ruleID, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("get archived status=%d want 404", resp.StatusCode)
	}
	_ = clk
}

func TestBilling_ArchivedAfterEntryRuleStillGeneratesFee(t *testing.T) {
	h, _, clk := newClockServer(t)
	area := mustAreaHTTP(t, h, 5)
	ruleID := mustBillingRuleHTTP(t, h, area, 0, 500, 4000)
	res := mustResidentHTTP(t, h, "1栋")
	start := *clk
	_, data := doJSON(t, h, http.MethodPost, "/api/v1/authorizations", map[string]interface{}{
		"resident_id": res, "parking_area_id": area, "plate": "京A12345",
		"start_time": start.Format(time.RFC3339), "end_time": start.Add(48 * time.Hour).Format(time.RFC3339),
	})
	var authEnv struct {
		Data *model.Authorization `json:"data"`
	}
	json.Unmarshal(data, &authEnv)
	resp, data := doJSON(t, h, http.MethodPost, "/api/v1/authorizations/"+authEnv.Data.ID+"/entry", nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("entry status=%d body=%s", resp.StatusCode, string(data))
	}

	advance(clk, time.Hour)
	resp, data = doJSON(t, h, http.MethodDelete, "/api/v1/billing-rules/"+ruleID, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("archive rule status=%d body=%s", resp.StatusCode, string(data))
	}
	advance(clk, time.Hour)
	resp, data = doJSON(t, h, http.MethodPost, "/api/v1/authorizations/"+authEnv.Data.ID+"/exit",
		map[string]interface{}{"operator": "guard1"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("exit status=%d body=%s", resp.StatusCode, string(data))
	}
	var exitEnv struct {
		Data *model.EntryExitRecord `json:"data"`
	}
	json.Unmarshal(data, &exitEnv)

	_, data = doJSON(t, h, http.MethodGet, "/api/v1/fees?record_id="+exitEnv.Data.ID, nil)
	var feeEnv struct {
		Data struct {
			Items []*model.Fee `json:"items"`
			Total int          `json:"total"`
		} `json:"data"`
	}
	json.Unmarshal(data, &feeEnv)
	if feeEnv.Data.Total != 1 || len(feeEnv.Data.Items) != 1 {
		t.Fatalf("expected fee from entry-time rule, got %s", string(data))
	}
	if feeEnv.Data.Items[0].BillingRuleID != ruleID || feeEnv.Data.Items[0].AmountCents != 1000 {
		t.Fatalf("fee = %+v, want rule %s and amount 1000", feeEnv.Data.Items[0], ruleID)
	}
}

func TestBillingRule_ValidationErrors(t *testing.T) {
	h, _, _ := newClockServer(t)
	area := mustAreaHTTP(t, h, 5)
	// missing area
	resp, data := doJSON(t, h, http.MethodPost, "/api/v1/billing-rules", map[string]interface{}{
		"free_minutes": 10, "hourly_rate_cents": 500, "daily_cap_cents": 1000,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing area status=%d body=%s", resp.StatusCode, string(data))
	}
	// zero rate
	resp, data = doJSON(t, h, http.MethodPost, "/api/v1/billing-rules", map[string]interface{}{
		"parking_area_id": area, "free_minutes": 10, "hourly_rate_cents": 0, "daily_cap_cents": 1000,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("zero rate status=%d body=%s", resp.StatusCode, string(data))
	}
	// negative free
	resp, data = doJSON(t, h, http.MethodPost, "/api/v1/billing-rules", map[string]interface{}{
		"parking_area_id": area, "free_minutes": -1, "hourly_rate_cents": 500, "daily_cap_cents": 1000,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("negative free status=%d body=%s", resp.StatusCode, string(data))
	}
	// archived area
	resp, data = doJSON(t, h, http.MethodDelete, "/api/v1/areas/"+area, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("archive area status=%d", resp.StatusCode)
	}
	resp, data = doJSON(t, h, http.MethodPost, "/api/v1/billing-rules", map[string]interface{}{
		"parking_area_id": area, "free_minutes": 10, "hourly_rate_cents": 500, "daily_cap_cents": 1000,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("archived area status=%d want 400 body=%s", resp.StatusCode, string(data))
	}
}

func TestBilling_NoRuleNoFee(t *testing.T) {
	h, _, clk := newClockServer(t)
	area := mustAreaHTTP(t, h, 5)
	// no billing rule for area
	res := mustResidentHTTP(t, h, "1栋")
	start := *clk
	end := start.Add(48 * time.Hour)
	_, data := doJSON(t, h, http.MethodPost, "/api/v1/authorizations", map[string]interface{}{
		"resident_id": res, "parking_area_id": area, "plate": "京A12345",
		"start_time": start.Format(time.RFC3339), "end_time": end.Format(time.RFC3339),
	})
	var authEnv struct {
		Data *model.Authorization `json:"data"`
	}
	json.Unmarshal(data, &authEnv)
	doJSON(t, h, http.MethodPost, "/api/v1/authorizations/"+authEnv.Data.ID+"/entry", nil)
	advance(clk, 2*time.Hour)
	resp, edata := doJSON(t, h, http.MethodPost, "/api/v1/authorizations/"+authEnv.Data.ID+"/exit",
		map[string]interface{}{"operator": "guard1"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("exit without rule status=%d body=%s", resp.StatusCode, string(edata))
	}
	// no fee should exist
	_, fdata := doJSON(t, h, http.MethodGet, "/api/v1/fees?status=unsettled", nil)
	var feeEnv struct {
		Data struct {
			Total int `json:"total"`
		} `json:"data"`
	}
	json.Unmarshal(fdata, &feeEnv)
	if feeEnv.Data.Total != 0 {
		t.Fatalf("expected 0 fees without rule, got %d", feeEnv.Data.Total)
	}
}

func TestBilling_RuleVersioning(t *testing.T) {
	h, _, clk := newClockServer(t)
	area := mustAreaHTTP(t, h, 5)
	// old rule effective now-1h with low rate
	_, data := doJSON(t, h, http.MethodPost, "/api/v1/billing-rules", map[string]interface{}{
		"parking_area_id":   area,
		"free_minutes":      0,
		"hourly_rate_cents": 300,
		"daily_cap_cents":   100000,
		"effective_from":    clk.Add(-1 * time.Hour).Format(time.RFC3339),
	})
	if statusOf(data) >= 400 {
		t.Fatalf("old rule create: %s", string(data))
	}
	// newer rule effective now with high rate
	_, data = doJSON(t, h, http.MethodPost, "/api/v1/billing-rules", map[string]interface{}{
		"parking_area_id":   area,
		"free_minutes":      0,
		"hourly_rate_cents": 900,
		"daily_cap_cents":   100000,
		"effective_from":    clk.Format(time.RFC3339),
	})
	if statusOf(data) >= 400 {
		t.Fatalf("new rule create: %s", string(data))
	}
	// entry happens now (effective_from == now selects the newer, higher-rate rule)
	res := mustResidentHTTP(t, h, "1栋")
	start := *clk
	end := start.Add(48 * time.Hour)
	_, data = doJSON(t, h, http.MethodPost, "/api/v1/authorizations", map[string]interface{}{
		"resident_id": res, "parking_area_id": area, "plate": "京A12345",
		"start_time": start.Format(time.RFC3339), "end_time": end.Format(time.RFC3339),
	})
	var authEnv struct {
		Data *model.Authorization `json:"data"`
	}
	json.Unmarshal(data, &authEnv)
	doJSON(t, h, http.MethodPost, "/api/v1/authorizations/"+authEnv.Data.ID+"/entry", nil)
	advance(clk, 2*time.Hour)
	_, edata := doJSON(t, h, http.MethodPost, "/api/v1/authorizations/"+authEnv.Data.ID+"/exit",
		map[string]interface{}{"operator": "guard1"})
	var exitEnv struct {
		Data *model.EntryExitRecord `json:"data"`
	}
	json.Unmarshal(edata, &exitEnv)
	_, fdata := doJSON(t, h, http.MethodGet, "/api/v1/fees?record_id="+exitEnv.Data.ID, nil)
	var feeEnv struct {
		Data struct {
			Items []*model.Fee `json:"items"`
		} `json:"data"`
	}
	json.Unmarshal(fdata, &feeEnv)
	if len(feeEnv.Data.Items) != 1 {
		t.Fatalf("expected 1 fee: %s", string(fdata))
	}
	// 2h * 900 = 1800 (newer rule selected)
	if feeEnv.Data.Items[0].AmountCents != 1800 {
		t.Fatalf("amount=%d want 1800 (newer rule)", feeEnv.Data.Items[0].AmountCents)
	}
}

func statusOf(data []byte) int {
	var env struct {
		Code int `json:"code"`
	}
	json.Unmarshal(data, &env)
	return env.Code
}

func TestSettleFee_ConcurrentOnlyOneWins(t *testing.T) {
	h, _, clk := newClockServer(t)
	fee, _ := fullExitAndFee(t, h, clk, 30, 500, 4000, 2*time.Hour)
	var wg sync.WaitGroup
	var mu sync.Mutex
	success, conflict := 0, 0
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, _ := doJSON(t, h, http.MethodPost, "/api/v1/fees/"+fee.ID+"/settle",
				map[string]interface{}{"method": "online", "operator": "c"})
			mu.Lock()
			defer mu.Unlock()
			if resp.StatusCode == http.StatusOK {
				success++
			} else if resp.StatusCode == http.StatusConflict {
				conflict++
			}
		}()
	}
	wg.Wait()
	if success != 1 {
		t.Fatalf("expected exactly 1 successful settle, got %d", success)
	}
	if conflict != 9 {
		t.Fatalf("expected 9 conflicts, got %d", conflict)
	}
}

func TestBilling_ExitReleasesCapacity(t *testing.T) {
	h, _, clk := newClockServer(t)
	area := mustAreaHTTP(t, h, 1) // capacity 1
	mustBillingRuleHTTP(t, h, area, 0, 500, 4000)
	res := mustResidentHTTP(t, h, "1栋")
	start := *clk
	end := start.Add(48 * time.Hour)
	mkauth := func(plate string) string {
		_, data := doJSON(t, h, http.MethodPost, "/api/v1/authorizations", map[string]interface{}{
			"resident_id": res, "parking_area_id": area, "plate": plate,
			"start_time": start.Format(time.RFC3339), "end_time": end.Format(time.RFC3339),
		})
		var env struct {
			Data *model.Authorization `json:"data"`
		}
		json.Unmarshal(data, &env)
		return env.Data.ID
	}
	a1 := mkauth("京A11111")
	doJSON(t, h, http.MethodPost, "/api/v1/authorizations/"+a1+"/entry", nil)
	// second vehicle cannot enter (capacity full)
	a2 := mkauth("京A22222")
	resp, _ := doJSON(t, h, http.MethodPost, "/api/v1/authorizations/"+a2+"/entry", nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("second entry status=%d want 409 (full)", resp.StatusCode)
	}
	// exit a1 releases the spot + generates fee in same tx
	advance(clk, time.Hour)
	resp, _ = doJSON(t, h, http.MethodPost, "/api/v1/authorizations/"+a1+"/exit",
		map[string]interface{}{"operator": "g"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("exit status=%d", resp.StatusCode)
	}
	// now a2 can enter
	resp, _ = doJSON(t, h, http.MethodPost, "/api/v1/authorizations/"+a2+"/entry", nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("second entry after exit status=%d want 201", resp.StatusCode)
	}
}
