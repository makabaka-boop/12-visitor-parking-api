package store

import (
	"context"
	"testing"
	"time"
	"visitor-parking/internal/model"
)

// TestApproveExtension_IgnoresExpiredPendingOverlap is the targeted reproduction
// for the bug where the approval-time same-plate overlap check counted a
// historical pending authorization whose validity window had already passed
// (end time earlier than the approval time) as a conflict, causing a valid
// extension application to be wrongly rejected with ErrConflict (HTTP 409).
//
// The authorizations are inserted directly into the memory store so that the
// approval overlap check can be exercised in isolation: a pending
// authorization whose window has fully elapsed is no longer live and must not
// be counted as an overlap, even though its stored status has not been
// transitioned to "expired" (which is a derived status).
func TestApproveExtension_IgnoresExpiredPendingOverlap(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	base := time.Date(2026, 8, 16, 10, 0, 0, 0, time.UTC)

	// prerequisites: an active resident and a parking area
	res := &model.Resident{ID: "res1", Name: "张三", Phone: "13800000000", Building: "1栋", Status: model.ResidentActive, CreatedAt: base, UpdatedAt: base}
	if err := m.CreateResident(ctx, res); err != nil {
		t.Fatalf("create resident: %v", err)
	}
	area := &model.ParkingArea{ID: "area1", Name: "A区", Code: "A", Capacity: 5, CreatedAt: base, UpdatedAt: base}
	if err := m.CreateParkingArea(ctx, area); err != nil {
		t.Fatalf("create area: %v", err)
	}

	// Auth A: the authorization being extended. Active, window [10:00, 15:00].
	authA := &model.Authorization{
		ID: "authA", ResidentID: "res1", Plate: "京A12345", ParkingAreaID: "area1",
		StartTime: base, EndTime: base.Add(5 * time.Hour), Status: model.AuthStatusActive,
		CreatedAt: base, UpdatedAt: base,
	}
	m.auths[authA.ID] = cloneAuth(authA)

	// Auth B: a historical pending authorization for the same plate. Its window
	// [10:30, 11:30] has already fully passed by the approval time (12:00), but
	// its stored status is still "pending" (expired is derived, not stored). It
	// overlaps the extended window [10:00, 17:00], yet being expired it must not
	// be treated as a conflict.
	authB := &model.Authorization{
		ID: "authB", ResidentID: "res1", Plate: "京A12345", ParkingAreaID: "area1",
		StartTime: base.Add(30 * time.Minute), EndTime: base.Add(90 * time.Minute), Status: model.AuthStatusPending,
		CreatedAt: base, UpdatedAt: base,
	}
	m.auths[authB.ID] = cloneAuth(authB)

	// approval at 12:00: auth A still within its window (15:00), auth B already
	// expired (11:30 < 12:00).
	now := base.Add(2 * time.Hour)
	newEnd := base.Add(7 * time.Hour) // extend A to 17:00

	app := &model.ExtensionApplication{
		ID: "ext1", AuthorizationID: "authA", Plate: "京A12345",
		OriginalEndTime: authA.EndTime, NewEndTime: newEnd,
		Reason: "访客需多停留", Applicant: "小王", Status: model.ExtStatusPending,
		CreatedAt: base, UpdatedAt: base,
	}
	m.extApps[app.ID] = cloneExtApp(app)

	_, _, err := m.ApproveExtensionApplication(ctx, app.ID, now, "mgr", "同意延期")
	if err != nil {
		t.Fatalf("approve should succeed; expired pending authorization must not be counted as an overlap: %v", err)
	}

	// confirm the authorization end time was actually extended
	got, err := m.GetAuthorization(ctx, "authA")
	if err != nil {
		t.Fatalf("get auth A: %v", err)
	}
	if !got.EndTime.Equal(newEnd) {
		t.Fatalf("auth A end_time=%v want %v", got.EndTime, newEnd)
	}
}
