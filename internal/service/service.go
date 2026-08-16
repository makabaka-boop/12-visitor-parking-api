package service
import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"visitor-parking/internal/model"
	"visitor-parking/internal/plate"
	"visitor-parking/internal/store"
)
const MaxAuthDuration = 7 * 24 * time.Hour
const ExpiringSoonWindow = 6 * time.Hour
type Service struct {
	store store.Store
	now   func() time.Time // injectable clock for deterministic tests
}
func New(s store.Store) *Service {
	return &Service{store: s, now: time.Now}
}
func NewWithClock(s store.Store, now func() time.Time) *Service {
	return &Service{store: s, now: now}
}
func (s *Service) nowT() time.Time { return s.now() }
func normPage(limit, offset int) model.Page {
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	return model.Page{Limit: limit, Offset: offset}
}
type CreateResidentInput struct {
	Name     string `json:"name"`
	Phone    string `json:"phone"`
	Building string `json:"building"`
	Unit     string `json:"unit"`
	Room     string `json:"room"`
	Status   string `json:"status"`
}
func (s *Service) CreateResident(ctx context.Context, in CreateResidentInput) (*model.Resident, error) {
	if err := validateResidentFields(in.Name, in.Phone, in.Building); err != nil {
		return nil, err
	}
	now := s.nowT()
	r := &model.Resident{
		ID:        store.NewID("res"),
		Name:      strings.TrimSpace(in.Name),
		Phone:     strings.TrimSpace(in.Phone),
		Building:  strings.TrimSpace(in.Building),
		Unit:      strings.TrimSpace(in.Unit),
		Room:      strings.TrimSpace(in.Room),
		Status:    defStatus(in.Status, model.ResidentActive),
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.store.CreateResident(ctx, r); err != nil {
		return nil, err
	}
	s.audit(ctx, "resident.create", "resident", r.ID, "system", "resident created")
	return r, nil
}
func validateResidentFields(name, phone, building string) error {
	if strings.TrimSpace(name) == "" {
		return newFieldError("name", "must not be empty")
	}
	if strings.TrimSpace(phone) == "" {
		return newFieldError("phone", "must not be empty")
	}
	if strings.TrimSpace(building) == "" {
		return newFieldError("building", "must not be empty")
	}
	return nil
}
func (s *Service) GetResident(ctx context.Context, id string) (*model.Resident, error) {
	return s.store.GetResident(ctx, id)
}
type UpdateResidentInput struct {
	Name     string `json:"name"`
	Phone    string `json:"phone"`
	Building string `json:"building"`
	Unit     string `json:"unit"`
	Room     string `json:"room"`
	Status   string `json:"status"`
	UpdatedAt time.Time `json:"updated_at"`
}
func (s *Service) UpdateResident(ctx context.Context, id string, in UpdateResidentInput) (*model.Resident, error) {
	if err := validateResidentFields(in.Name, in.Phone, in.Building); err != nil {
		return nil, err
	}
	if in.Status != "" && in.Status != model.ResidentActive && in.Status != model.ResidentDisabled {
		return nil, newFieldError("status", "must be active or disabled")
	}
	if in.UpdatedAt.IsZero() {
		return nil, newFieldError("updated_at", "is required for concurrency check")
	}
	cur, err := s.store.GetResident(ctx, id)
	if err != nil {
		return nil, err
	}
	if !in.UpdatedAt.Equal(cur.UpdatedAt) {
		return nil, store.ErrConcurrentModify
	}
	now := s.nowT()
	cur.Name = strings.TrimSpace(in.Name)
	cur.Phone = strings.TrimSpace(in.Phone)
	cur.Building = strings.TrimSpace(in.Building)
	cur.Unit = strings.TrimSpace(in.Unit)
	cur.Room = strings.TrimSpace(in.Room)
	if in.Status != "" {
		cur.Status = in.Status
	}
	cur.UpdatedAt = now
	if err := s.store.UpdateResident(ctx, cur); err != nil {
		return nil, err
	}
	s.audit(ctx, "resident.update", "resident", id, "system", "resident updated")
	return cur, nil
}
func (s *Service) ArchiveResident(ctx context.Context, id string) error {
	if err := s.store.ArchiveResident(ctx, id, s.nowT()); err != nil {
		return err
	}
	s.audit(ctx, "resident.archive", "resident", id, "system", "resident archived")
	return nil
}
func (s *Service) ListResidents(ctx context.Context, limit, offset int) ([]*model.Resident, int64, error) {
	return s.store.ListResidents(ctx, normPage(limit, offset))
}
type CreateVehicleInput struct {
	Plate      string `json:"plate"`
	OwnerName  string `json:"owner_name"`
	OwnerPhone string `json:"owner_phone"`
	Color      string `json:"color"`
}
func (s *Service) CreateVehicle(ctx context.Context, in CreateVehicleInput) (*model.Vehicle, error) {
	p := plate.Normalize(in.Plate)
	if !plate.Valid(p) {
		return nil, newFieldError("plate", "invalid plate format")
	}
	now := s.nowT()
	v := &model.Vehicle{
		ID:         store.NewID("veh"),
		Plate:      p,
		OwnerName:  strings.TrimSpace(in.OwnerName),
		OwnerPhone: strings.TrimSpace(in.OwnerPhone),
		Color:      strings.TrimSpace(in.Color),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.store.CreateVehicle(ctx, v); err != nil {
		return nil, err
	}
	s.audit(ctx, "vehicle.create", "vehicle", v.ID, "system", "vehicle created")
	return v, nil
}
func (s *Service) GetVehicle(ctx context.Context, id string) (*model.Vehicle, error) {
	return s.store.GetVehicle(ctx, id)
}
func (s *Service) UpdateVehicle(ctx context.Context, id string, v *model.Vehicle) (*model.Vehicle, error) {
	if v.UpdatedAt.IsZero() {
		return nil, newFieldError("updated_at", "is required for concurrency check")
	}
	if v.Plate != "" {
		p := plate.Normalize(v.Plate)
		if !plate.Valid(p) {
			return nil, newFieldError("plate", "invalid plate format")
		}
		v.Plate = p
	}
	cur, err := s.store.GetVehicle(ctx, id)
	if err != nil {
		return nil, err
	}
	if !v.UpdatedAt.Equal(cur.UpdatedAt) {
		return nil, store.ErrConcurrentModify
	}
	if v.OwnerName != "" {
		cur.OwnerName = v.OwnerName
	}
	if v.OwnerPhone != "" {
		cur.OwnerPhone = v.OwnerPhone
	}
	if v.Color != "" {
		cur.Color = v.Color
	}
	if v.Plate != "" {
		cur.Plate = v.Plate
	}
	cur.UpdatedAt = s.nowT()
	if err := s.store.UpdateVehicle(ctx, cur); err != nil {
		return nil, err
	}
	s.audit(ctx, "vehicle.update", "vehicle", id, "system", "vehicle updated")
	return cur, nil
}
func (s *Service) ArchiveVehicle(ctx context.Context, id string) error {
	if err := s.store.ArchiveVehicle(ctx, id, s.nowT()); err != nil {
		return err
	}
	s.audit(ctx, "vehicle.archive", "vehicle", id, "system", "vehicle archived")
	return nil
}
func (s *Service) ListVehicles(ctx context.Context, limit, offset int) ([]*model.Vehicle, int64, error) {
	return s.store.ListVehicles(ctx, normPage(limit, offset))
}
type CreateParkingAreaInput struct {
	Name     string `json:"name"`
	Code     string `json:"code"`
	Capacity int    `json:"capacity"`
}
func (s *Service) CreateParkingArea(ctx context.Context, in CreateParkingAreaInput) (*model.ParkingArea, error) {
	if strings.TrimSpace(in.Name) == "" {
		return nil, newFieldError("name", "must not be empty")
	}
	if strings.TrimSpace(in.Code) == "" {
		return nil, newFieldError("code", "must not be empty")
	}
	if in.Capacity <= 0 {
		return nil, newFieldError("capacity", "must be greater than zero")
	}
	now := s.nowT()
	a := &model.ParkingArea{
		ID:        store.NewID("area"),
		Name:      strings.TrimSpace(in.Name),
		Code:      strings.ToUpper(strings.TrimSpace(in.Code)),
		Capacity:  in.Capacity,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.store.CreateParkingArea(ctx, a); err != nil {
		return nil, err
	}
	s.audit(ctx, "area.create", "parking_area", a.ID, "system", "parking area created")
	return a, nil
}
func (s *Service) GetParkingArea(ctx context.Context, id string) (*model.ParkingArea, error) {
	return s.store.GetParkingArea(ctx, id)
}
func (s *Service) UpdateParkingArea(ctx context.Context, id string, in *model.ParkingArea) (*model.ParkingArea, error) {
	if in.UpdatedAt.IsZero() {
		return nil, newFieldError("updated_at", "is required for concurrency check")
	}
	cur, err := s.store.GetParkingArea(ctx, id)
	if err != nil {
		return nil, err
	}
	if !in.UpdatedAt.Equal(cur.UpdatedAt) {
		return nil, store.ErrConcurrentModify
	}
	if in.Name != "" {
		cur.Name = in.Name
	}
	if in.Code != "" {
		cur.Code = strings.ToUpper(in.Code)
	}
	if in.Capacity > 0 {
		cur.Capacity = in.Capacity
	}
	cur.UpdatedAt = s.nowT()
	if err := s.store.UpdateParkingArea(ctx, cur); err != nil {
		return nil, err
	}
	s.audit(ctx, "area.update", "parking_area", id, "system", "parking area updated")
	return cur, nil
}
func (s *Service) ArchiveParkingArea(ctx context.Context, id string) error {
	if err := s.store.ArchiveParkingArea(ctx, id, s.nowT()); err != nil {
		return err
	}
	s.audit(ctx, "area.archive", "parking_area", id, "system", "parking area archived")
	return nil
}
func (s *Service) ListParkingAreas(ctx context.Context, limit, offset int) ([]*model.ParkingArea, int64, error) {
	return s.store.ListParkingAreas(ctx, normPage(limit, offset))
}
type CreateAuthorizationInput struct {
	ResidentID    string    `json:"resident_id"`
	Plate         string    `json:"plate"`
	ParkingAreaID string    `json:"parking_area_id"`
	StartTime     time.Time `json:"start_time"`
	EndTime       time.Time `json:"end_time"`
	Purpose       string    `json:"purpose"`
	CreatedBy     string    `json:"created_by"`
	Confirmer     string    `json:"confirmer"` // required only when the plate is under a manual_confirm restriction
}
func (s *Service) CreateAuthorization(ctx context.Context, in CreateAuthorizationInput) (*model.Authorization, error) {
	now := s.nowT()
	p := plate.Normalize(in.Plate)
	if !plate.Valid(p) {
		return nil, newFieldError("plate", "invalid plate format")
	}
	if in.StartTime.IsZero() || in.EndTime.IsZero() {
		return nil, newFieldError("start_time", "start_time and end_time are required")
	}
	if in.StartTime.Before(now) {
		return nil, newFieldError("start_time", "must not be earlier than current time")
	}
	if !in.EndTime.After(in.StartTime) {
		return nil, newFieldError("end_time", "must be later than start_time")
	}
	if in.EndTime.Sub(in.StartTime) > MaxAuthDuration {
		return nil, newFieldError("end_time", "authorization duration must not exceed 7 days")
	}
	a := &model.Authorization{
		ID:            store.NewID("auth"),
		ResidentID:    in.ResidentID,
		Plate:         p,
		ParkingAreaID: in.ParkingAreaID,
		StartTime:     in.StartTime,
		EndTime:       in.EndTime,
		Status:        model.AuthStatusPending,
		Purpose:       strings.TrimSpace(in.Purpose),
		CreatedBy:     defStr(in.CreatedBy, "system"),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	matched, err := s.store.CreateAuthorization(ctx, a, now, strings.TrimSpace(in.Confirmer))
	if matched != nil || err == nil {
		s.auditRestrictionCheck(ctx, "authorization.create", a.Plate, defStr(strings.TrimSpace(in.Confirmer), a.CreatedBy), matched, strings.TrimSpace(in.Confirmer), err)
	}
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "authorization.create", "authorization", a.ID, a.CreatedBy, "authorization created")
	return a, nil
}
func (s *Service) GetAuthorization(ctx context.Context, id string) (*model.Authorization, error) {
	return s.store.GetAuthorization(ctx, id)
}
func (s *Service) UpdateAuthorization(ctx context.Context, id string, in *model.Authorization) (*model.Authorization, error) {
	cur, err := s.store.GetAuthorization(ctx, id)
	if err != nil {
		return nil, err
	}
	if cur.Status != model.AuthStatusPending {
		return nil, store.ErrStatusTransition
	}
	now := s.nowT()
	if !in.UpdatedAt.Equal(cur.UpdatedAt) {
		return nil, store.ErrConcurrentModify
	}
	if in.StartTime.IsZero() {
		in.StartTime = cur.StartTime
	}
	if in.EndTime.IsZero() {
		in.EndTime = cur.EndTime
	}
	if in.StartTime.Before(now) {
		return nil, newFieldError("start_time", "must not be earlier than current time")
	}
	if !in.EndTime.After(in.StartTime) {
		return nil, newFieldError("end_time", "must be later than start_time")
	}
	if in.EndTime.Sub(in.StartTime) > MaxAuthDuration {
		return nil, newFieldError("end_time", "authorization duration must not exceed 7 days")
	}
	cur.StartTime = in.StartTime
	cur.EndTime = in.EndTime
	if in.Purpose != "" {
		cur.Purpose = in.Purpose
	}
	if in.ParkingAreaID != "" {
		cur.ParkingAreaID = in.ParkingAreaID
	}
	cur.UpdatedAt = now
	if err := s.store.UpdateAuthorization(ctx, cur); err != nil {
		return nil, err
	}
	s.audit(ctx, "authorization.update", "authorization", id, "system", "authorization updated")
	return cur, nil
}
func (s *Service) ArchiveAuthorization(ctx context.Context, id string) error {
	if err := s.store.ArchiveAuthorization(ctx, id, s.nowT()); err != nil {
		return err
	}
	s.audit(ctx, "authorization.archive", "authorization", id, "system", "authorization archived")
	return nil
}
func (s *Service) ListAuthorizations(ctx context.Context, f ListAuthFilter) ([]*model.Authorization, int64, error) {
	now := s.nowT()
	storeFilter := model.AuthFilter{
		Building:      f.Building,
		Plate:         plate.Normalize(f.Plate),
		ParkingAreaID: f.ParkingAreaID,
		Status:        f.Status,
		Page:          normPage(f.Limit, f.Offset),
		ValidOnNow:    &now,
	}
	if f.ValidOn != "" {
		t, err := time.Parse(time.RFC3339, f.ValidOn)
		if err == nil {
			storeFilter.ValidOn = &t
		}
	}
	if f.Today {
		y, m, d := now.Date()
		start := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
		end := start.Add(24 * time.Hour)
		storeFilter.StartFrom = &start
		storeFilter.StartTo = &end
	}
	if f.ExpiringSoon {
		cutoff := now.Add(ExpiringSoonWindow)
		storeFilter.EndingBefore = &cutoff
	}
	return s.store.ListAuthorizations(ctx, storeFilter)
}
type ListAuthFilter struct {
	Building      string
	Plate         string
	ParkingAreaID string
	Status        string
	ValidOn       string // RFC3339
	Today         bool
	ExpiringSoon  bool
	Limit, Offset int
}
func (s *Service) EnterVehicle(ctx context.Context, authID, confirmer string) (*model.EntryExitRecord, error) {
	now := s.nowT()
	confirmer = strings.TrimSpace(confirmer)
	rec, matched, err := s.store.EnterVehicle(ctx, authID, now, confirmer)
	if matched != nil || err == nil {
		plate := ""
		if rec != nil {
			plate = rec.Plate
		} else if matched != nil {
			plate = matched.Plate
		}
		s.auditRestrictionCheck(ctx, "entry", plate, defStr(confirmer, "gate"), matched, confirmer, err)
	}
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "entry.record", "record", rec.ID, defStr(confirmer, "gate"), fmt.Sprintf("vehicle %s entered area %s", rec.Plate, rec.ParkingAreaID))
	return rec, nil
}
type ExitRequest struct {
	Operator string `json:"operator"`
	Note     string `json:"note"`
}
func (s *Service) ExitVehicle(ctx context.Context, authID string, in ExitRequest) (*model.EntryExitRecord, error) {
	now := s.nowT()
	rec, err := s.store.ExitVehicle(ctx, authID, now, strings.TrimSpace(in.Operator), in.Note)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "exit.record", "record", rec.ID, defStr(in.Operator, "system"), fmt.Sprintf("vehicle %s exited", rec.Plate))
	return rec, nil
}
func (s *Service) Revoke(ctx context.Context, authID string, operator, reason string) (*model.Authorization, error) {
	a, err := s.store.RevokeAuthorization(ctx, authID, s.nowT(), operator, reason)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "authorization.revoke", "authorization", authID, operator, "revoked: "+reason)
	return a, nil
}
func (s *Service) ListRecords(ctx context.Context, areaID, status, plate string, limit, offset int) ([]*model.EntryExitRecord, int64, error) {
	return s.store.ListRecords(ctx, store.RecordFilter{
		AreaID: areaID, Status: status, Plate: plate, Page: normPage(limit, offset),
	})
}
func (s *Service) ListCurrentVehicles(ctx context.Context, areaID string, limit, offset int) ([]*model.EntryExitRecord, int64, error) {
	return s.store.ListCurrentVehicles(ctx, areaID, normPage(limit, offset))
}
func (s *Service) TodayArrivals(ctx context.Context, limit, offset int) ([]*model.Authorization, int64, error) {
	return s.ListAuthorizations(ctx, ListAuthFilter{Today: true, Limit: limit, Offset: offset})
}
func (s *Service) ExpiringSoon(ctx context.Context, limit, offset int) ([]*model.Authorization, int64, error) {
	return s.ListAuthorizations(ctx, ListAuthFilter{ExpiringSoon: true, Limit: limit, Offset: offset})
}
func (s *Service) AreaOccupancy(ctx context.Context) ([]*model.AreaOccupancy, error) {
	return s.store.AreaOccupancy(ctx)
}
func (s *Service) ListAuditLogs(ctx context.Context, entityType string, limit, offset int) ([]*model.AuditLog, error) {
	logs, _, err := s.store.ListAuditLogs(ctx, entityType, normPage(limit, offset))
	return logs, err
}
func (s *Service) audit(ctx context.Context, action, etype, eid, operator, detail string) {
	_ = s.store.CreateAuditLog(ctx, &model.AuditLog{
		ID: store.NewID("log"), Action: action, EntityType: etype, EntityID: eid,
		Operator: defStr(operator, "system"), Detail: detail, CreatedAt: s.nowT(),
	})
}
func defStatus(in, def string) string {
	if in == "" {
		return def
	}
	return in
}
func defStr(in, def string) string {
	if in == "" {
		return def
	}
	return in
}
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}
func (e *FieldError) Error() string { return e.Field + ": " + e.Message }
func newFieldError(field, msg string) *FieldError { return &FieldError{Field: field, Message: msg} }
func NewFieldError(field, msg string) *FieldError { return &FieldError{Field: field, Message: msg} }
func IsFieldError(err error) bool {
	var fe *FieldError
	return errors.As(err, &fe)
}
func FieldErrorOf(err error) *FieldError {
	var fe *FieldError
	if errors.As(err, &fe) {
		return fe
	}
	return nil
}
