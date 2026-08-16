//go:build pg

// This file is excluded from the default `go test ./...` run (build tag `pg`).
// It exercises the PostgreSQL store against a real database to validate the
// SQL in migrations 002 and the billing transaction paths that the in-memory
// tests cannot reach. Run with:
//
//	TEST_DSN="postgres://postgres:postgres@localhost:5432/visitor_parking_test?sslmode=disable" \
//	  go test -tags=pg ./internal/httpd/ -run TestPG -v
package httpd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
	"visitor-parking/internal/migrations"
	"visitor-parking/internal/model"
	"visitor-parking/internal/service"
	"visitor-parking/internal/store"
)

func TestPG_BillingFullFlow(t *testing.T) {
	dsn := os.Getenv("TEST_DSN")
	if dsn == "" {
		t.Skip("TEST_DSN not set")
	}
	pg, err := store.NewPostgres(dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pg.Close()
	db := pg.DB()
	if err := migrations.Apply(context.Background(), db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	for _, tbl := range []string{"fees", "billing_rules", "entry_exit_records", "audit_logs", "authorizations", "vehicles", "parking_areas", "residents"} {
		if _, err := db.ExecContext(context.Background(), fmt.Sprintf("DELETE FROM %s", tbl)); err != nil {
			t.Fatalf("clean %s: %v", tbl, err)
		}
	}

	clk := time.Date(2026, 8, 16, 10, 0, 0, 0, time.UTC)
	svc := service.NewWithClock(pg, func() time.Time { return clk })
	h := NewHandler(svc).Routes()

	post := func(path string, body interface{}) (int, []byte) {
		var req *http.Request
		if body != nil {
			b, _ := json.Marshal(body)
			req = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
			req.Header.Set("Content-Type", "application/json")
		} else {
			req = httptest.NewRequest(http.MethodPost, path, nil)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w.Code, w.Body.Bytes()
	}
	get := func(path string) (int, []byte) {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w.Code, w.Body.Bytes()
	}

	code, data := post("/api/v1/areas", map[string]interface{}{"name": "A", "code": "APGA", "capacity": 5})
	var areaEnv struct{ Data *model.ParkingArea }
	json.Unmarshal(data, &areaEnv)
	if code != 201 {
		t.Fatalf("area %d %s", code, data)
	}
	area := areaEnv.Data.ID

	code, data = post("/api/v1/billing-rules", map[string]interface{}{
		"parking_area_id": area, "free_minutes": 30, "hourly_rate_cents": 500, "daily_cap_cents": 4000,
	})
	if code != 201 {
		t.Fatalf("rule %d %s", code, data)
	}

	code, data = post("/api/v1/residents", map[string]interface{}{"name": "Z", "phone": "13800000000", "building": "1", "unit": "1", "room": "1"})
	var resEnv struct{ Data *model.Resident }
	json.Unmarshal(data, &resEnv)
	if code != 201 {
		t.Fatalf("resident %d %s", code, data)
	}

	start := clk
	end := start.Add(48 * time.Hour)
	code, data = post("/api/v1/authorizations", map[string]interface{}{
		"resident_id": resEnv.Data.ID, "parking_area_id": area, "plate": "京A12345",
		"start_time": start.Format(time.RFC3339), "end_time": end.Format(time.RFC3339), "created_by": "s",
	})
	var authEnv struct{ Data *model.Authorization }
	json.Unmarshal(data, &authEnv)
	if code != 201 {
		t.Fatalf("auth %d %s", code, data)
	}

	if code, _ = post("/api/v1/authorizations/"+authEnv.Data.ID+"/entry", nil); code != 201 {
		t.Fatalf("entry %d", code)
	}
	clk = clk.Add(2 * time.Hour)

	code, data = post("/api/v1/authorizations/"+authEnv.Data.ID+"/exit", map[string]interface{}{"operator": "g"})
	if code != 200 {
		t.Fatalf("exit %d %s", code, data)
	}
	var exitEnv struct{ Data *model.EntryExitRecord }
	json.Unmarshal(data, &exitEnv)

	code, data = get("/api/v1/fees?record_id=" + exitEnv.Data.ID)
	if code != 200 {
		t.Fatalf("fees %d %s", code, data)
	}
	var feeEnv struct {
		Data struct {
			Items []*model.Fee
			Total int
		}
	}
	json.Unmarshal(data, &feeEnv)
	if feeEnv.Data.Total != 1 {
		t.Fatalf("fee total %d", feeEnv.Data.Total)
	}
	fee := feeEnv.Data.Items[0]
	if fee.AmountCents != 1000 {
		t.Fatalf("amount=%d want 1000", fee.AmountCents)
	}

	code, data = post("/api/v1/fees/"+fee.ID+"/settle", map[string]interface{}{"method": "cash", "operator": "c"})
	if code != 200 {
		t.Fatalf("settle %d %s", code, data)
	}
	code, _ = post("/api/v1/fees/"+fee.ID+"/settle", map[string]interface{}{"method": "online"})
	if code != 409 {
		t.Fatalf("re-settle %d want 409", code)
	}

	code, data = get("/api/v1/stats/area-revenue?area_id=" + area)
	if code != 200 {
		t.Fatalf("revenue %d %s", code, data)
	}
	var revEnv struct {
		Data struct {
			Areas []*model.AreaRevenue
		}
	}
	json.Unmarshal(data, &revEnv)
	if len(revEnv.Data.Areas) == 0 || revEnv.Data.Areas[0].SettledCents != 1000 {
		t.Fatalf("revenue: %s", data)
	}

	// cross-day: 18:00 day1 -> 06:00 day2 = 12h. rule cap is 4000 (not hit),
	// so each day charged separately: day1 6h->3000, day2 6h->3000, total 6000.
	clk = time.Date(2026, 8, 16, 18, 0, 0, 0, time.UTC)
	start = clk
	end = start.Add(48 * time.Hour)
	code, data = post("/api/v1/authorizations", map[string]interface{}{
		"resident_id": resEnv.Data.ID, "parking_area_id": area, "plate": "京B22222",
		"start_time": start.Format(time.RFC3339), "end_time": end.Format(time.RFC3339), "created_by": "s",
	})
	json.Unmarshal(data, &authEnv)
	if code != 201 {
		t.Fatalf("auth2 %d %s", code, data)
	}
	post("/api/v1/authorizations/"+authEnv.Data.ID+"/entry", nil)
	clk = clk.Add(12 * time.Hour)
	code, data = post("/api/v1/authorizations/"+authEnv.Data.ID+"/exit", map[string]interface{}{"operator": "g"})
	json.Unmarshal(data, &exitEnv)
	code, data = get("/api/v1/fees?record_id=" + exitEnv.Data.ID)
	json.Unmarshal(data, &feeEnv)
	if feeEnv.Data.Total != 1 {
		t.Fatalf("fee2 total %d", feeEnv.Data.Total)
	}
	if feeEnv.Data.Items[0].AmountCents != 6000 {
		t.Fatalf("cross-day amount=%d want 6000", feeEnv.Data.Items[0].AmountCents)
	}
	t.Logf("PG billing flow OK: settled fee=%s (%d cents), cross-day fee=%s (%d cents)",
		fee.ID, fee.AmountCents, feeEnv.Data.Items[0].ID, feeEnv.Data.Items[0].AmountCents)
}
