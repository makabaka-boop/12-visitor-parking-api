package store

import (
	"context"
	"os"
	"testing"
	"time"
	"visitor-parking/internal/migrations"
	"visitor-parking/internal/model"
)

// newTestPostgres connects to the dedicated test database (VP_TEST_DSN),
// applies migrations, and returns a ready-to-use Postgres store. The test is
// skipped when VP_TEST_DSN is unset so `go test ./...` still passes without a
// database. The caller is responsible for clearing per-table state.
func newTestPostgres(t *testing.T) *Postgres {
	t.Helper()
	dsn := os.Getenv("VP_TEST_DSN")
	if dsn == "" {
		t.Skip("VP_TEST_DSN not set; skipping PostgreSQL integration test")
	}
	ctx := context.Background()
	pg, err := NewPostgres(dsn)
	if err != nil {
		t.Fatalf("connect postgres: %v", err)
	}
	t.Cleanup(func() { pg.Close() })
	if err := migrations.Apply(ctx, pg.DB()); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	return pg
}

// clearRestrictions wipes the vehicle_restrictions table so each test starts
// from a known empty state (the table has no inbound foreign keys).
func clearRestrictions(t *testing.T, pg *Postgres) {
	t.Helper()
	if _, err := pg.DB().ExecContext(context.Background(), `DELETE FROM vehicle_restrictions`); err != nil {
		t.Fatalf("clear vehicle_restrictions: %v", err)
	}
}

func mkRestriction(id, plate, rtype string, from, to time.Time) *model.VehicleRestriction {
	return &model.VehicleRestriction{
		ID:            id,
		Plate:         plate,
		Type:          rtype,
		EffectiveFrom: from,
		EffectiveTo:   to,
		Reason:        "test",
		RegisteredBy:  "manager1",
		Status:        model.RestrictionStatusActive,
		CreatedAt:     from,
		UpdatedAt:     from,
	}
}

// TestPostgres_ListVehicleRestrictions_CombinedFilter is the regression test for
// the scan/structure mismatch in the PostgreSQL restriction-list query. The
// query selects count(*) OVER() plus the 11 restriction columns (12 total), but
// the row scan only read the 11 struct fields via scanRestriction and never
// captured the total — producing a real PostgreSQL scan error
// ("sql: expected 11 destination arguments in Scan, not 12") and an always-zero
// total whenever the paginated list endpoint returned any row.
func TestPostgres_ListVehicleRestrictions_CombinedFilter(t *testing.T) {
	pg := newTestPostgres(t)
	clearRestrictions(t, pg)
	ctx := context.Background()

	base := time.Date(2026, 8, 16, 10, 0, 0, 0, time.UTC)
	end := base.Add(24 * time.Hour)
	now := base

	// Three active restrictions: two forbidden (different plates) and one
	// manual_confirm. Inserted through the store so the overlap guard and
	// column order are exercised exactly as in production.
	seed := []*model.VehicleRestriction{
		mkRestriction("rstr-pg-1", "京A11111", model.RestrictionTypeForbidden, base, end),
		mkRestriction("rstr-pg-2", "京A22222", model.RestrictionTypeManualConfirm, base, end),
		mkRestriction("rstr-pg-3", "京A33333", model.RestrictionTypeForbidden, base, end),
	}
	for _, r := range seed {
		if err := pg.CreateVehicleRestriction(ctx, r, now); err != nil {
			t.Fatalf("seed restriction %s: %v", r.ID, err)
		}
	}

	// Combined filter: type=forbidden AND status=active. Must return the two
	// forbidden restrictions with no scan error and a correct total.
	items, total, err := pg.ListVehicleRestrictions(ctx, model.RestrictionFilter{
		Type:   model.RestrictionTypeForbidden,
		Status: model.RestrictionStatusActive,
		Page:   model.Page{Limit: 20, Offset: 0},
	})
	if err != nil {
		t.Fatalf("ListVehicleRestrictions (type+status) scan error: %v", err)
	}
	if total != 2 {
		t.Fatalf("type=forbidden&status=active total=%d want 2", total)
	}
	if len(items) != 2 {
		t.Fatalf("type=forbidden&status=active items=%d want 2", len(items))
	}
	for _, r := range items {
		if r.Type != model.RestrictionTypeForbidden {
			t.Fatalf("unexpected type %q in filtered results", r.Type)
		}
		if r.Status != model.RestrictionStatusActive {
			t.Fatalf("unexpected status %q in filtered results", r.Status)
		}
	}

	// Combined filter: plate + type + status. Narrows to exactly one row.
	items, total, err = pg.ListVehicleRestrictions(ctx, model.RestrictionFilter{
		Plate:  "京A11111",
		Type:   model.RestrictionTypeForbidden,
		Status: model.RestrictionStatusActive,
		Page:   model.Page{Limit: 20, Offset: 0},
	})
	if err != nil {
		t.Fatalf("ListVehicleRestrictions (plate+type+status) scan error: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].Plate != "京A11111" {
		t.Fatalf("plate+type+status filter: total=%d items=%v", total, items)
	}

	// Combined filter: effective_on point check + status. All three active
	// restrictions are in effect at base+1h, so the window predicate is also
	// exercised alongside the count column.
	t1 := base.Add(time.Hour)
	items, total, err = pg.ListVehicleRestrictions(ctx, model.RestrictionFilter{
		Status:      model.RestrictionStatusActive,
		EffectiveOn: &t1,
		Page:        model.Page{Limit: 20, Offset: 0},
	})
	if err != nil {
		t.Fatalf("ListVehicleRestrictions (effective_on+status) scan error: %v", err)
	}
	if total != 3 || len(items) != 3 {
		t.Fatalf("effective_on+status filter: total=%d items=%d want 3", total, len(items))
	}

	// Empty result set must also be handled cleanly (no rows, total=0).
	items, total, err = pg.ListVehicleRestrictions(ctx, model.RestrictionFilter{
		Plate:  "京Z99999",
		Status: model.RestrictionStatusActive,
		Page:   model.Page{Limit: 20, Offset: 0},
	})
	if err != nil {
		t.Fatalf("ListVehicleRestrictions (no match) error: %v", err)
	}
	if total != 0 || len(items) != 0 {
		t.Fatalf("no-match filter: total=%d items=%d want 0", total, len(items))
	}
}
