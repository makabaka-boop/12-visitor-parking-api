package store

import (
	"context"
	"testing"
	"time"
	"visitor-parking/internal/model"
)

func TestApproveExtensionIgnoresExpiredPendingOverlap(t *testing.T) {
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	m := NewMemory()
	auth := &model.Authorization{
		ID: "auth-current", Plate: "京A12345", Status: model.AuthStatusActive,
		StartTime: now.Add(-4 * time.Hour), EndTime: now.Add(time.Hour),
	}
	stale := &model.Authorization{
		ID: "auth-stale", Plate: auth.Plate, Status: model.AuthStatusPending,
		StartTime: now.Add(-3 * time.Hour), EndTime: now.Add(-time.Hour),
	}
	app := &model.ExtensionApplication{
		ID: "ext-1", AuthorizationID: auth.ID, Plate: auth.Plate,
		OriginalEndTime: auth.EndTime, NewEndTime: now.Add(2 * time.Hour),
		Status: model.ExtStatusPending, CreatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour),
	}
	m.auths[auth.ID] = auth
	m.auths[stale.ID] = stale
	m.extApps[app.ID] = app

	approved, updated, err := m.ApproveExtensionApplication(context.Background(), app.ID, now, "manager", "approved")
	if err != nil {
		t.Fatalf("ApproveExtensionApplication() error = %v", err)
	}
	if approved.Status != model.ExtStatusApproved {
		t.Fatalf("application status = %q, want %q", approved.Status, model.ExtStatusApproved)
	}
	if !updated.EndTime.Equal(app.NewEndTime) {
		t.Fatalf("authorization end time = %v, want %v", updated.EndTime, app.NewEndTime)
	}
}
