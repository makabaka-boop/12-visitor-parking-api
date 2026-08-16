package httpd

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
	"visitor-parking/internal/model"
)

// mustRestrictionHTTP creates a vehicle restriction and returns its id.
func mustRestrictionHTTP(t *testing.T, h http.Handler, plate, rtype string, from, to time.Time) string {
	t.Helper()
	_, data := doJSON(t, h, http.MethodPost, "/api/v1/restrictions", map[string]interface{}{
		"plate":          plate,
		"type":           rtype,
		"effective_from": from.Format(time.RFC3339),
		"effective_to":   to.Format(time.RFC3339),
		"reason":         "test",
		"registered_by":  "manager1",
	})
	var env struct {
		Data *model.VehicleRestriction `json:"data"`
	}
	if err := json.Unmarshal(data, &env); err != nil || env.Data == nil {
		t.Fatalf("create restriction: %s", string(data))
	}
	return env.Data.ID
}

// authBody builds a create-authorization body for the given plate.
func authBody(area, res, plate string, start, end time.Time) map[string]interface{} {
	return map[string]interface{}{
		"resident_id":     res,
		"parking_area_id": area,
		"plate":           plate,
		"start_time":      start.Format(time.RFC3339),
		"end_time":        end.Format(time.RFC3339),
		"created_by":      "staff1",
	}
}

func TestCreateRestriction_Validation(t *testing.T) {
	h, _ := newServer(t, 0)
	from := baseTime()
	to := from.Add(24 * time.Hour)
	cases := []struct {
		name string
		body map[string]interface{}
		want int
	}{
		{"bad plate", map[string]interface{}{"plate": "abc", "type": "forbidden", "effective_from": from.Format(time.RFC3339), "effective_to": to.Format(time.RFC3339)}, http.StatusBadRequest},
		{"bad type", map[string]interface{}{"plate": "京A11111", "type": "bogus", "effective_from": from.Format(time.RFC3339), "effective_to": to.Format(time.RFC3339)}, http.StatusBadRequest},
		{"to before from", map[string]interface{}{"plate": "京A11111", "type": "forbidden", "effective_from": to.Format(time.RFC3339), "effective_to": from.Format(time.RFC3339)}, http.StatusBadRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp, _ := doJSON(t, h, http.MethodPost, "/api/v1/restrictions", c.body)
			if resp.StatusCode != c.want {
				t.Fatalf("%s: status=%d want %d", c.name, resp.StatusCode, c.want)
			}
		})
	}
	// happy path
	resp, data := doJSON(t, h, http.MethodPost, "/api/v1/restrictions", map[string]interface{}{
		"plate": "京A22222", "type": "forbidden",
		"effective_from": from.Format(time.RFC3339), "effective_to": to.Format(time.RFC3339),
		"reason": "blacklist", "registered_by": "mgr",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", resp.StatusCode, string(data))
	}
	var env struct {
		Data *model.VehicleRestriction `json:"data"`
	}
	json.Unmarshal(data, &env)
	if env.Data.Status != model.RestrictionStatusActive || env.Data.Type != model.RestrictionTypeForbidden {
		t.Fatalf("create body: %s", string(data))
	}
}

func TestRestriction_OverlapGuard(t *testing.T) {
	h, _ := newServer(t, 0)
	from := baseTime()
	to := from.Add(24 * time.Hour)
	mustRestrictionHTTP(t, h, "京B11111", model.RestrictionTypeForbidden, from, to)
	// overlapping second restriction for same plate -> 409
	resp, _ := doJSON(t, h, http.MethodPost, "/api/v1/restrictions", map[string]interface{}{
		"plate": "京B11111", "type": model.RestrictionTypeForbidden,
		"effective_from": from.Add(12 * time.Hour).Format(time.RFC3339),
		"effective_to":   to.Add(12 * time.Hour).Format(time.RFC3339),
	})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("overlap status=%d want 409", resp.StatusCode)
	}
	// non-overlapping (after the first ends) -> 201
	resp, _ = doJSON(t, h, http.MethodPost, "/api/v1/restrictions", map[string]interface{}{
		"plate": "京B11111", "type": model.RestrictionTypeManualConfirm,
		"effective_from": to.Add(time.Hour).Format(time.RFC3339),
		"effective_to":   to.Add(2 * time.Hour).Format(time.RFC3339),
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("non-overlap status=%d want 201", resp.StatusCode)
	}
}

func TestCreateAuthorization_ForbiddenPlateRejected(t *testing.T) {
	h, _ := newServer(t, 0)
	area := mustAreaHTTP(t, h, 5)
	res := mustResidentHTTP(t, h, "1栋")
	from := baseTime()
	to := from.Add(24 * time.Hour)
	mustRestrictionHTTP(t, h, "京C11111", model.RestrictionTypeForbidden, from, to)
	start := from.Add(time.Hour)
	end := start.Add(5 * time.Hour)
	resp, data := doJSON(t, h, http.MethodPost, "/api/v1/authorizations", authBody(area, res, "京C11111", start, end))
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("forbidden plate auth status=%d body=%s", resp.StatusCode, string(data))
	}
	// audit log should record the denial
	_, ad := doJSON(t, h, http.MethodGet, "/api/v1/audit-logs?entity_type=restriction", nil)
	var aenv struct {
		Data struct {
			Items []*model.AuditLog `json:"items"`
		} `json:"data"`
	}
	json.Unmarshal(ad, &aenv)
	found := false
	for _, l := range aenv.Data.Items {
		if l.Action == "restriction.check" && strings.Contains(l.Detail, "forbidden") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected restriction.check forbidden audit log, got %s", string(ad))
	}
}

func TestCreateAuthorization_ManualConfirmRequiresConfirmer(t *testing.T) {
	h, _ := newServer(t, 0)
	area := mustAreaHTTP(t, h, 5)
	res := mustResidentHTTP(t, h, "1栋")
	from := baseTime()
	to := from.Add(24 * time.Hour)
	mustRestrictionHTTP(t, h, "京D11111", model.RestrictionTypeManualConfirm, from, to)
	start := from.Add(time.Hour)
	end := start.Add(5 * time.Hour)
	// without confirmer -> 400
	resp, _ := doJSON(t, h, http.MethodPost, "/api/v1/authorizations", authBody(area, res, "京D11111", start, end))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("manual confirm no confirmer status=%d want 400", resp.StatusCode)
	}
	// with confirmer -> 201
	body := authBody(area, res, "京D11111", start, end)
	body["confirmer"] = "guard2"
	resp, data := doJSON(t, h, http.MethodPost, "/api/v1/authorizations", body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("manual confirm with confirmer status=%d body=%s", resp.StatusCode, string(data))
	}
	// audit log should mention the confirmer
	_, ad := doJSON(t, h, http.MethodGet, "/api/v1/audit-logs?entity_type=restriction", nil)
	var aenv struct {
		Data struct {
			Items []*model.AuditLog `json:"items"`
		} `json:"data"`
	}
	json.Unmarshal(ad, &aenv)
	found := false
	for _, l := range aenv.Data.Items {
		if l.Action == "restriction.check" && strings.Contains(l.Detail, "guard2") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected restriction.check audit log mentioning confirmer, got %s", string(ad))
	}
}

func TestEntry_RestrictionEnforced(t *testing.T) {
	// Clock is fixed at baseTime(). To exercise the entry-time restriction
	// check we first create an authorization whose window covers `now` (so the
	// auth is created while no restriction exists), then add a restriction that
	// covers `now` and attempt entry. Entry checks the restriction at the
	// entry instant (`now`).
	h, _ := newServer(t, 0)
	area := mustAreaHTTP(t, h, 5)
	res := mustResidentHTTP(t, h, "1栋")
	from := baseTime()
	to := from.Add(24 * time.Hour)

	t.Run("forbidden_blocks_entry", func(t *testing.T) {
		// auth window covers now; created before the restriction exists.
		as := from
		ae := from.Add(5 * time.Hour)
		_, dd := doJSON(t, h, http.MethodPost, "/api/v1/authorizations", authBody(area, res, "京E22222", as, ae))
		var e2 struct {
			Data *model.Authorization `json:"data"`
		}
		json.Unmarshal(dd, &e2)
		if e2.Data == nil {
			t.Fatalf("create auth: %s", string(dd))
		}
		mustRestrictionHTTP(t, h, "京E22222", model.RestrictionTypeForbidden, from, to)
		resp, _ := doJSON(t, h, http.MethodPost, "/api/v1/authorizations/"+e2.Data.ID+"/entry", nil)
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("forbidden entry status=%d want 403", resp.StatusCode)
		}
	})
	t.Run("manual_confirm_requires_confirmer_on_entry", func(t *testing.T) {
		as := from
		ae := from.Add(5 * time.Hour)
		_, dd := doJSON(t, h, http.MethodPost, "/api/v1/authorizations", authBody(area, res, "京E33333", as, ae))
		var e3 struct {
			Data *model.Authorization `json:"data"`
		}
		json.Unmarshal(dd, &e3)
		if e3.Data == nil {
			t.Fatalf("create auth: %s", string(dd))
		}
		mustRestrictionHTTP(t, h, "京E33333", model.RestrictionTypeManualConfirm, from, to)
		// no confirmer -> 400
		resp, _ := doJSON(t, h, http.MethodPost, "/api/v1/authorizations/"+e3.Data.ID+"/entry", nil)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("manual entry no confirmer status=%d want 400", resp.StatusCode)
		}
		// with confirmer -> 201
		resp, bd := doJSON(t, h, http.MethodPost, "/api/v1/authorizations/"+e3.Data.ID+"/entry",
			map[string]interface{}{"confirmer": "guard9"})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("manual entry with confirmer status=%d body=%s", resp.StatusCode, string(bd))
		}
	})
}

func TestReleaseRestriction_KeepsHistoryAndAllowsEntry(t *testing.T) {
	h, _ := newServer(t, 0)
	area := mustAreaHTTP(t, h, 5)
	res := mustResidentHTTP(t, h, "1栋")
	from := baseTime()
	to := from.Add(24 * time.Hour)
	rid := mustRestrictionHTTP(t, h, "京F11111", model.RestrictionTypeForbidden, from, to)
	// release it
	resp, data := doJSON(t, h, http.MethodPost, "/api/v1/restrictions/"+rid+"/release",
		map[string]interface{}{"operator": "mgr", "reason": "false alarm"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("release status=%d body=%s", resp.StatusCode, string(data))
	}
	var env struct {
		Data *model.VehicleRestriction `json:"data"`
	}
	json.Unmarshal(data, &env)
	if env.Data.Status != model.RestrictionStatusReleased {
		t.Fatalf("released status=%s", env.Data.Status)
	}
	// still visible in list (history retained)
	_, ld := doJSON(t, h, http.MethodGet, "/api/v1/restrictions?plate=京F11111", nil)
	var lenv struct {
		Data struct {
			Items []*model.VehicleRestriction `json:"items"`
			Total int                         `json:"total"`
		} `json:"data"`
	}
	json.Unmarshal(ld, &lenv)
	if lenv.Data.Total != 1 {
		t.Fatalf("expected released restriction still listed, total=%d", lenv.Data.Total)
	}
	// now an auth for that plate can be created
	start := from.Add(time.Hour)
	end := start.Add(5 * time.Hour)
	resp, ad := doJSON(t, h, http.MethodPost, "/api/v1/authorizations", authBody(area, res, "京F11111", start, end))
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("auth after release status=%d body=%s", resp.StatusCode, string(ad))
	}
}

func TestArchiveRestriction_SoftDelete(t *testing.T) {
	h, _ := newServer(t, 0)
	from := baseTime()
	to := from.Add(24 * time.Hour)
	rid := mustRestrictionHTTP(t, h, "京G11111", model.RestrictionTypeForbidden, from, to)
	resp, _ := doJSON(t, h, http.MethodDelete, "/api/v1/restrictions/"+rid, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("archive status=%d", resp.StatusCode)
	}
	resp, _ = doJSON(t, h, http.MethodGet, "/api/v1/restrictions/"+rid, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("get archived status=%d want 404", resp.StatusCode)
	}
}

func TestUpdateRestriction_ConcurrencyAndOverlap(t *testing.T) {
	h, _ := newServer(t, 0)
	from := baseTime()
	to := from.Add(24 * time.Hour)
	rid := mustRestrictionHTTP(t, h, "京H11111", model.RestrictionTypeManualConfirm, from, to)
	_, gd := doJSON(t, h, http.MethodGet, "/api/v1/restrictions/"+rid, nil)
	var env struct {
		Data *model.VehicleRestriction `json:"data"`
	}
	json.Unmarshal(gd, &env)
	// stale updated_at -> 409
	resp, _ := doJSON(t, h, http.MethodPut, "/api/v1/restrictions/"+rid, map[string]interface{}{
		"type":           "manual_confirm",
		"effective_from": from.Add(-1 * time.Hour).Format(time.RFC3339),
		"effective_to":   to.Format(time.RFC3339),
		"updated_at":     env.Data.UpdatedAt.Add(-time.Second).Format(time.RFC3339Nano),
	})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("stale update status=%d want 409", resp.StatusCode)
	}
	// fresh updated_at -> 200
	resp, _ = doJSON(t, h, http.MethodPut, "/api/v1/restrictions/"+rid, map[string]interface{}{
		"type":           "manual_confirm",
		"effective_from": from.Add(-1 * time.Hour).Format(time.RFC3339),
		"effective_to":   to.Format(time.RFC3339),
		"updated_at":     env.Data.UpdatedAt.Format(time.RFC3339Nano),
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("fresh update status=%d want 200", resp.StatusCode)
	}
}

func TestListRestrictions_FilterAndStats(t *testing.T) {
	h, _ := newServer(t, 0)
	from := baseTime()
	to := from.Add(24 * time.Hour)
	mustRestrictionHTTP(t, h, "京I11111", model.RestrictionTypeForbidden, from, to)
	mustRestrictionHTTP(t, h, "京I22222", model.RestrictionTypeManualConfirm, from, to)
	// filter by type
	resp, data := doJSON(t, h, http.MethodGet, "/api/v1/restrictions?type=forbidden", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list status=%d", resp.StatusCode)
	}
	var env struct {
		Data struct {
			Items []*model.VehicleRestriction `json:"items"`
			Total int                         `json:"total"`
		} `json:"data"`
	}
	json.Unmarshal(data, &env)
	if env.Data.Total != 1 || env.Data.Items[0].Type != model.RestrictionTypeForbidden {
		t.Fatalf("filter by type: %s", string(data))
	}
	// stats
	resp, data = doJSON(t, h, http.MethodGet, "/api/v1/stats/restrictions", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stats status=%d", resp.StatusCode)
	}
	var st struct {
		Data *model.RestrictionStats `json:"data"`
	}
	json.Unmarshal(data, &st)
	if st.Data.TotalActive != 2 || st.Data.Forbidden != 1 || st.Data.ManualConfirm != 1 || st.Data.CurrentlyInEffect != 2 {
		t.Fatalf("stats: %s", string(data))
	}
}

// TestRestrictionOverlap_Concurrent verifies the overlap guard is race-free:
// many goroutines try to create an overlapping active restriction for the same
// plate/window; exactly one must succeed.
func TestRestrictionOverlap_Concurrent(t *testing.T) {
	h, _ := newServer(t, 0)
	from := baseTime()
	to := from.Add(24 * time.Hour)
	const n = 16
	var wg sync.WaitGroup
	wg.Add(n)
	results := make([]int, n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			resp, _ := doJSON(t, h, http.MethodPost, "/api/v1/restrictions", map[string]interface{}{
				"plate": "京J11111", "type": model.RestrictionTypeForbidden,
				"effective_from": from.Format(time.RFC3339),
				"effective_to":   to.Format(time.RFC3339),
				"registered_by":  "mgr",
			})
			results[idx] = resp.StatusCode
		}(i)
	}
	wg.Wait()
	created := 0
	for _, s := range results {
		if s == http.StatusCreated {
			created++
		}
	}
	if created != 1 {
		t.Fatalf("expected exactly 1 created, got %d (statuses=%v)", created, results)
	}
}
