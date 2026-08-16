package store
import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
	"visitor-parking/internal/model"
)
type Memory struct {
	mu        sync.Mutex
	residents map[string]*model.Resident
	vehicles  map[string]*model.Vehicle
	areas     map[string]*model.ParkingArea
	auths     map[string]*model.Authorization
	records   map[string]*model.EntryExitRecord
	extApps   map[string]*model.ExtensionApplication
	logs      map[string]*model.AuditLog
}
func NewMemory() *Memory {
	return &Memory{
		residents: make(map[string]*model.Resident),
		vehicles:  make(map[string]*model.Vehicle),
		areas:     make(map[string]*model.ParkingArea),
		auths:     make(map[string]*model.Authorization),
		records:   make(map[string]*model.EntryExitRecord),
		extApps:   make(map[string]*model.ExtensionApplication),
		logs:      make(map[string]*model.AuditLog),
	}
}
func clonePtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	c := *t
	return &c
}
func cloneResident(r *model.Resident) *model.Resident {
	c := *r
	c.ArchivedAt = clonePtr(r.ArchivedAt)
	return &c
}
func cloneVehicle(v *model.Vehicle) *model.Vehicle {
	c := *v
	c.ArchivedAt = clonePtr(v.ArchivedAt)
	return &c
}
func cloneArea(a *model.ParkingArea) *model.ParkingArea {
	c := *a
	c.ArchivedAt = clonePtr(a.ArchivedAt)
	return &c
}
func cloneAuth(a *model.Authorization) *model.Authorization {
	c := *a
	c.ArchivedAt = clonePtr(a.ArchivedAt)
	return &c
}
func cloneRecord(r *model.EntryExitRecord) *model.EntryExitRecord {
	c := *r
	c.ExitTime = clonePtr(r.ExitTime)
	return &c
}
func cloneExtApp(a *model.ExtensionApplication) *model.ExtensionApplication {
	c := *a
	c.DecidedAt = clonePtr(a.DecidedAt)
	return &c
}
func normPage(p model.Page) model.Page {
	if p.Limit <= 0 || p.Limit > 200 {
		p.Limit = 20
	}
	if p.Offset < 0 {
		p.Offset = 0
	}
	return p
}
func pageOf[T any](items []T, p model.Page) []T {
	p = normPage(p)
	if p.Offset >= len(items) {
		return nil
	}
	end := p.Offset + p.Limit
	if end > len(items) {
		end = len(items)
	}
	return items[p.Offset:end]
}
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func (m *Memory) CreateResident(ctx context.Context, r *model.Resident) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.residents[r.ID]; ok {
		return ErrConflict
	}
	m.residents[r.ID] = cloneResident(r)
	return nil
}
func (m *Memory) GetResident(ctx context.Context, id string) (*model.Resident, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.residents[id]
	if !ok || r.ArchivedAt != nil {
		return nil, ErrNotFound
	}
	return cloneResident(r), nil
}
func (m *Memory) UpdateResident(ctx context.Context, r *model.Resident) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.residents[r.ID]
	if !ok || cur.ArchivedAt != nil {
		return ErrNotFound
	}
	if !cur.UpdatedAt.Equal(r.UpdatedAt) {
		return ErrConcurrentModify
	}
	m.residents[r.ID] = cloneResident(r)
	return nil
}
func (m *Memory) ArchiveResident(ctx context.Context, id string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.residents[id]
	if !ok || r.ArchivedAt != nil {
		return ErrNotFound
	}
	c := now
	r.ArchivedAt = &c
	r.UpdatedAt = now
	return nil
}
func (m *Memory) ListResidents(ctx context.Context, page model.Page) ([]*model.Resident, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*model.Resident, 0, len(m.residents))
	for _, r := range m.residents {
		if r.ArchivedAt == nil {
			out = append(out, cloneResident(r))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return pageOf(out, page), int64(len(out)), nil
}
func (m *Memory) CreateVehicle(ctx context.Context, v *model.Vehicle) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.vehicles[v.ID]; ok {
		return ErrConflict
	}
	m.vehicles[v.ID] = cloneVehicle(v)
	return nil
}
func (m *Memory) GetVehicle(ctx context.Context, id string) (*model.Vehicle, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.vehicles[id]
	if !ok || v.ArchivedAt != nil {
		return nil, ErrNotFound
	}
	return cloneVehicle(v), nil
}
func (m *Memory) GetVehicleByPlate(ctx context.Context, plate string) (*model.Vehicle, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, v := range m.vehicles {
		if v.Plate == plate && v.ArchivedAt == nil {
			return cloneVehicle(v), nil
		}
	}
	return nil, ErrNotFound
}
func (m *Memory) UpdateVehicle(ctx context.Context, v *model.Vehicle) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.vehicles[v.ID]
	if !ok || cur.ArchivedAt != nil {
		return ErrNotFound
	}
	if !cur.UpdatedAt.Equal(v.UpdatedAt) {
		return ErrConcurrentModify
	}
	m.vehicles[v.ID] = cloneVehicle(v)
	return nil
}
func (m *Memory) ArchiveVehicle(ctx context.Context, id string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.vehicles[id]
	if !ok || v.ArchivedAt != nil {
		return ErrNotFound
	}
	c := now
	v.ArchivedAt = &c
	v.UpdatedAt = now
	return nil
}
func (m *Memory) ListVehicles(ctx context.Context, page model.Page) ([]*model.Vehicle, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*model.Vehicle, 0, len(m.vehicles))
	for _, v := range m.vehicles {
		if v.ArchivedAt == nil {
			out = append(out, cloneVehicle(v))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return pageOf(out, page), int64(len(out)), nil
}
func (m *Memory) CreateParkingArea(ctx context.Context, a *model.ParkingArea) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.areas[a.ID]; ok {
		return ErrConflict
	}
	m.areas[a.ID] = cloneArea(a)
	return nil
}
func (m *Memory) GetParkingArea(ctx context.Context, id string) (*model.ParkingArea, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.areas[id]
	if !ok || a.ArchivedAt != nil {
		return nil, ErrNotFound
	}
	return cloneArea(a), nil
}
func (m *Memory) UpdateParkingArea(ctx context.Context, a *model.ParkingArea) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.areas[a.ID]
	if !ok || cur.ArchivedAt != nil {
		return ErrNotFound
	}
	if !cur.UpdatedAt.Equal(a.UpdatedAt) {
		return ErrConcurrentModify
	}
	m.areas[a.ID] = cloneArea(a)
	return nil
}
func (m *Memory) ArchiveParkingArea(ctx context.Context, id string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.areas[id]
	if !ok || a.ArchivedAt != nil {
		return ErrNotFound
	}
	c := now
	a.ArchivedAt = &c
	a.UpdatedAt = now
	return nil
}
func (m *Memory) ListParkingAreas(ctx context.Context, page model.Page) ([]*model.ParkingArea, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*model.ParkingArea, 0, len(m.areas))
	for _, a := range m.areas {
		if a.ArchivedAt == nil {
			out = append(out, cloneArea(a))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return pageOf(out, page), int64(len(out)), nil
}
func (m *Memory) CreateAuthorization(ctx context.Context, a *model.Authorization, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.residents[a.ResidentID]
	if !ok || r.ArchivedAt != nil {
		return ErrNotFound
	}
	if r.Status != model.ResidentActive {
		return ErrResidentDisabled
	}
	area, ok := m.areas[a.ParkingAreaID]
	if !ok || area.ArchivedAt != nil {
		return ErrAreaArchived
	}
	for _, ex := range m.auths {
		if ex.ArchivedAt != nil || ex.Plate != a.Plate {
			continue
		}
		if ex.Status == model.AuthStatusCompleted || ex.Status == model.AuthStatusCancelled {
			continue
		}
		if a.StartTime.Before(ex.EndTime) && ex.StartTime.Before(a.EndTime) {
			return ErrConflict
		}
	}
	m.auths[a.ID] = cloneAuth(a)
	return nil
}
func (m *Memory) GetAuthorization(ctx context.Context, id string) (*model.Authorization, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.auths[id]
	if !ok || a.ArchivedAt != nil {
		return nil, ErrNotFound
	}
	return cloneAuth(a), nil
}
func (m *Memory) UpdateAuthorization(ctx context.Context, a *model.Authorization) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.auths[a.ID]
	if !ok || cur.ArchivedAt != nil {
		return ErrNotFound
	}
	if !cur.UpdatedAt.Equal(a.UpdatedAt) {
		return ErrConcurrentModify
	}
	m.auths[a.ID] = cloneAuth(a)
	return nil
}
func (m *Memory) ArchiveAuthorization(ctx context.Context, id string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.auths[id]
	if !ok || a.ArchivedAt != nil {
		return ErrNotFound
	}
	c := now
	a.ArchivedAt = &c
	a.UpdatedAt = now
	return nil
}
func deriveStatus(a *model.Authorization, now *time.Time) string {
	if a.Status == model.AuthStatusPending && now != nil && !a.EndTime.After(*now) {
		return model.AuthStatusExpired
	}
	return a.Status
}
func (m *Memory) ListAuthorizations(ctx context.Context, f model.AuthFilter) ([]*model.Authorization, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var buildingIDs map[string]bool
	if f.Building != "" {
		buildingIDs = make(map[string]bool)
		for _, r := range m.residents {
			if r.ArchivedAt == nil && r.Building == f.Building {
				buildingIDs[r.ID] = true
			}
		}
	}
	out := make([]*model.Authorization, 0, len(m.auths))
	for _, a := range m.auths {
		if a.ArchivedAt != nil {
			continue
		}
		if f.Plate != "" && a.Plate != f.Plate {
			continue
		}
		if f.ParkingAreaID != "" && a.ParkingAreaID != f.ParkingAreaID {
			continue
		}
		if f.Status != "" {
			if deriveStatus(a, f.ValidOnNow) != f.Status {
				continue
			}
		} else if f.ValidOn != nil && !(a.StartTime.Before(*f.ValidOn) && a.EndTime.After(*f.ValidOn)) {
			continue
		}
		if f.StartFrom != nil && !a.StartTime.After(*f.StartFrom) {
			continue
		}
		if f.StartTo != nil && !a.StartTime.Before(*f.StartTo) {
			continue
		}
		if f.EndingBefore != nil && !a.EndTime.Before(*f.EndingBefore) {
			continue
		}
		if buildingIDs != nil && !buildingIDs[a.ResidentID] {
			continue
		}
		out = append(out, cloneAuth(a))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartTime.After(out[j].StartTime) })
	return pageOf(out, f.Page), int64(len(out)), nil
}
func (m *Memory) CreateRecord(ctx context.Context, r *model.EntryExitRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.records[r.ID]; ok {
		return ErrConflict
	}
	m.records[r.ID] = cloneRecord(r)
	return nil
}
func (m *Memory) GetRecord(ctx context.Context, id string) (*model.EntryExitRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.records[id]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneRecord(r), nil
}
func (m *Memory) ListRecords(ctx context.Context, f RecordFilter) ([]*model.EntryExitRecord, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*model.EntryExitRecord, 0, len(m.records))
	for _, r := range m.records {
		if f.AreaID != "" && r.ParkingAreaID != f.AreaID {
			continue
		}
		if f.Status != "" && r.Status != f.Status {
			continue
		}
		if f.Plate != "" && r.Plate != f.Plate {
			continue
		}
		out = append(out, cloneRecord(r))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EntryTime.After(out[j].EntryTime) })
	return pageOf(out, f.Page), int64(len(out)), nil
}
func (m *Memory) CreateAuditLog(ctx context.Context, l *model.AuditLog) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := *l
	m.logs[l.ID] = &c
	return nil
}
func (m *Memory) ListAuditLogs(ctx context.Context, entityType string, page model.Page) ([]*model.AuditLog, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*model.AuditLog, 0, len(m.logs))
	for _, l := range m.logs {
		if entityType != "" && l.EntityType != entityType {
			continue
		}
		c := *l
		out = append(out, &c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return pageOf(out, page), int64(len(out)), nil
}
func (m *Memory) ListCurrentVehicles(ctx context.Context, areaID string, page model.Page) ([]*model.EntryExitRecord, int64, error) {
	return m.ListRecords(ctx, RecordFilter{AreaID: areaID, Status: model.RecordStatusEntered, Page: page})
}
func (m *Memory) AreaOccupancy(ctx context.Context) ([]*model.AreaOccupancy, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*model.AreaOccupancy, 0, len(m.areas))
	for _, a := range m.areas {
		if a.ArchivedAt != nil {
			continue
		}
		occupied := 0
		for _, rec := range m.records {
			if rec.ParkingAreaID == a.ID && rec.Status == model.RecordStatusEntered {
				occupied++
			}
		}
		util := 0.0
		if a.Capacity > 0 {
			util = float64(occupied) / float64(a.Capacity)
		}
		out = append(out, &model.AreaOccupancy{AreaID: a.ID, AreaName: a.Name, Code: a.Code, Capacity: a.Capacity, Occupied: occupied, Available: max(0, a.Capacity-occupied), Utilization: util})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Utilization > out[j].Utilization })
	return out, nil
}
func (m *Memory) EnterVehicle(ctx context.Context, authID string, now time.Time) (*model.EntryExitRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.auths[authID]
	if !ok || a.ArchivedAt != nil {
		return nil, ErrNotFound
	}
	if a.Status != model.AuthStatusPending {
		if a.Status == model.AuthStatusActive {
			return nil, ErrAlreadyEntered
		}
		return nil, ErrStatusTransition
	}
	if now.Before(a.StartTime) || !now.Before(a.EndTime) {
		return nil, ErrOutOfTimeWindow
	}
	area, ok := m.areas[a.ParkingAreaID]
	if !ok || area.ArchivedAt != nil {
		return nil, ErrAreaArchived
	}
	occupied := 0
	for _, rec := range m.records {
		if rec.ParkingAreaID == area.ID && rec.Status == model.RecordStatusEntered {
			occupied++
		}
	}
	if occupied >= area.Capacity {
		return nil, ErrNoCapacity
	}
	rec := &model.EntryExitRecord{ID: NewID("rec"), AuthorizationID: authID, Plate: a.Plate, ParkingAreaID: a.ParkingAreaID, EntryTime: now, Status: model.RecordStatusEntered, CreatedAt: now, UpdatedAt: now}
	m.records[rec.ID] = cloneRecord(rec)
	a.Status = model.AuthStatusActive
	a.UpdatedAt = now
	m.auths[authID] = cloneAuth(a)
	return rec, nil
}
func (m *Memory) ExitVehicle(ctx context.Context, authID string, now time.Time, operator, note string) (*model.EntryExitRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.auths[authID]
	if !ok || a.ArchivedAt != nil {
		return nil, ErrNotFound
	}
	if a.Status == model.AuthStatusCompleted {
		return nil, ErrAlreadyExited
	}
	if a.Status != model.AuthStatusActive {
		return nil, ErrStatusTransition
	}
	var rec *model.EntryExitRecord
	for _, r := range m.records {
		if r.AuthorizationID == authID && r.Status == model.RecordStatusEntered {
			rec = r
			break
		}
	}
	if rec == nil {
		return nil, ErrStatusTransition
	}
	et := now
	rec.ExitTime = &et
	rec.ExitOperator = operator
	rec.ExitNote = note
	rec.Status = model.RecordStatusExited
	rec.UpdatedAt = now
	m.records[rec.ID] = cloneRecord(rec)
	a.Status = model.AuthStatusCompleted
	a.UpdatedAt = now
	m.auths[authID] = cloneAuth(a)
	return cloneRecord(rec), nil
}
func (m *Memory) RevokeAuthorization(ctx context.Context, authID string, now time.Time, operator, reason string) (*model.Authorization, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.auths[authID]
	if !ok || a.ArchivedAt != nil {
		return nil, ErrNotFound
	}
	if a.Status == model.AuthStatusActive {
		return nil, fmt.Errorf("%w: active authorization must be exited first", ErrStatusTransition)
	}
	if a.Status != model.AuthStatusPending {
		return nil, ErrStatusTransition
	}
	a.Status = model.AuthStatusCancelled
	a.UpdatedAt = now
	m.auths[authID] = cloneAuth(a)
	return cloneAuth(a), nil
}
func (m *Memory) CreateExtensionApplication(ctx context.Context, app *model.ExtensionApplication, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.auths[app.AuthorizationID]
	if !ok || a.ArchivedAt != nil {
		return ErrNotFound
	}
	if !model.ExtensibleAuth(a.Status, a.EndTime, now) {
		return ErrNotExtensible
	}
	for _, ex := range m.extApps {
		if ex.AuthorizationID == app.AuthorizationID && ex.Status == model.ExtStatusPending {
			return ErrConflict
		}
	}
	app.Plate = a.Plate
	app.OriginalEndTime = a.EndTime
	m.extApps[app.ID] = cloneExtApp(app)
	return nil
}
func (m *Memory) GetExtensionApplication(ctx context.Context, id string) (*model.ExtensionApplication, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	app, ok := m.extApps[id]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneExtApp(app), nil
}
// ApproveExtensionApplication mutates the authorization's end_time and the
// application status under the same lock, re-checking eligibility, the 7-day
// cap and plate overlap. The audit log is written here (within the logical
// transaction) so it commits atomically with the state change.
func (m *Memory) ApproveExtensionApplication(ctx context.Context, appID string, now time.Time, approver, note string) (*model.ExtensionApplication, *model.Authorization, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	app, ok := m.extApps[appID]
	if !ok {
		return nil, nil, ErrNotFound
	}
	if app.Status != model.ExtStatusPending {
		return nil, nil, ErrStatusTransition
	}
	a, ok := m.auths[app.AuthorizationID]
	if !ok || a.ArchivedAt != nil {
		return nil, nil, ErrNotFound
	}
	if !model.ExtensibleAuth(a.Status, a.EndTime, now) {
		return nil, nil, ErrNotExtensible
	}
	if !app.NewEndTime.After(a.EndTime) {
		return nil, nil, fmt.Errorf("%w: new end time must be later than current authorization end time", ErrConflict)
	}
	if app.NewEndTime.Sub(a.StartTime) > model.MaxAuthDuration {
		return nil, nil, fmt.Errorf("%w: total authorization duration must not exceed 7 days", ErrConflict)
	}
	for _, ex := range m.auths {
		if ex.ArchivedAt != nil || ex.ID == a.ID || ex.Plate != a.Plate {
			continue
		}
		if ex.Status == model.AuthStatusCompleted || ex.Status == model.AuthStatusCancelled {
			continue
		}
		if a.StartTime.Before(ex.EndTime) && ex.StartTime.Before(app.NewEndTime) {
			return nil, nil, ErrConflict
		}
	}
	a.EndTime = app.NewEndTime
	a.UpdatedAt = now
	m.auths[a.ID] = cloneAuth(a)
	decided := now
	app.Status = model.ExtStatusApproved
	app.DecidedBy = approver
	app.DecidedAt = &decided
	app.DecisionNote = note
	app.UpdatedAt = now
	m.extApps[app.ID] = cloneExtApp(app)
	m.logs[NewID("log")] = &model.AuditLog{
		Action: "extension.approve", EntityType: "extension_application", EntityID: app.ID,
		Operator: approver, Detail: fmt.Sprintf("approved; authorization %s end_time extended to %s", a.ID, app.NewEndTime.Format(time.RFC3339)), CreatedAt: now,
	}
	return cloneExtApp(app), cloneAuth(a), nil
}
func (m *Memory) RejectExtensionApplication(ctx context.Context, appID string, now time.Time, approver, reason string) (*model.ExtensionApplication, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	app, ok := m.extApps[appID]
	if !ok {
		return nil, ErrNotFound
	}
	if app.Status != model.ExtStatusPending {
		return nil, ErrStatusTransition
	}
	decided := now
	app.Status = model.ExtStatusRejected
	app.DecidedBy = approver
	app.DecidedAt = &decided
	app.DecisionNote = reason
	app.UpdatedAt = now
	m.extApps[app.ID] = cloneExtApp(app)
	return cloneExtApp(app), nil
}
func (m *Memory) RevokeExtensionApplication(ctx context.Context, appID string, now time.Time, operator, reason string) (*model.ExtensionApplication, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	app, ok := m.extApps[appID]
	if !ok {
		return nil, ErrNotFound
	}
	if app.Status != model.ExtStatusPending {
		return nil, ErrStatusTransition
	}
	decided := now
	app.Status = model.ExtStatusRevoked
	app.DecidedBy = operator
	app.DecidedAt = &decided
	app.DecisionNote = reason
	app.UpdatedAt = now
	m.extApps[app.ID] = cloneExtApp(app)
	return cloneExtApp(app), nil
}
func (m *Memory) ListExtensionApplications(ctx context.Context, f model.ExtensionAppFilter) ([]*model.ExtensionApplication, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*model.ExtensionApplication, 0, len(m.extApps))
	for _, app := range m.extApps {
		if f.AuthorizationID != "" && app.AuthorizationID != f.AuthorizationID {
			continue
		}
		if f.Plate != "" && app.Plate != f.Plate {
			continue
		}
		if f.Status != "" && app.Status != f.Status {
			continue
		}
		if f.Applicant != "" && app.Applicant != f.Applicant {
			continue
		}
		out = append(out, cloneExtApp(app))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return pageOf(out, f.Page), int64(len(out)), nil
}
