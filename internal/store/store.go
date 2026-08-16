package store
import (
	"context"
	"errors"
	"time"
	"visitor-parking/internal/model"
)
var (
	ErrNotFound         = errors.New("not found")
	ErrConflict         = errors.New("conflict")                // duplicate/overlap
	ErrConcurrentModify = errors.New("concurrent modification") // optimistic-lock mismatch
	ErrStatusTransition = errors.New("invalid status transition")
	ErrNoCapacity       = errors.New("parking area has no available capacity")
	ErrAlreadyEntered   = errors.New("vehicle already entered")
	ErrAlreadyExited    = errors.New("vehicle already exited")
	ErrAuthNotUsable    = errors.New("authorization not in a usable state")
	ErrOutOfTimeWindow  = errors.New("authorization is outside its valid time window")
	ErrResidentDisabled = errors.New("resident is disabled")
	ErrAreaArchived     = errors.New("parking area is archived")
)
type RecordFilter struct {
	AreaID string
	Status string // entered | exited
	Plate  string
	Page   model.Page
}
type Store interface {
	CreateResident(ctx context.Context, r *model.Resident) error
	GetResident(ctx context.Context, id string) (*model.Resident, error)
	UpdateResident(ctx context.Context, r *model.Resident) error // optimistic lock via updated_at
	ArchiveResident(ctx context.Context, id string, now time.Time) error
	ListResidents(ctx context.Context, page model.Page) ([]*model.Resident, int64, error)
	CreateVehicle(ctx context.Context, v *model.Vehicle) error
	GetVehicle(ctx context.Context, id string) (*model.Vehicle, error)
	GetVehicleByPlate(ctx context.Context, plate string) (*model.Vehicle, error)
	UpdateVehicle(ctx context.Context, v *model.Vehicle) error
	ArchiveVehicle(ctx context.Context, id string, now time.Time) error
	ListVehicles(ctx context.Context, page model.Page) ([]*model.Vehicle, int64, error)
	CreateParkingArea(ctx context.Context, a *model.ParkingArea) error
	GetParkingArea(ctx context.Context, id string) (*model.ParkingArea, error)
	UpdateParkingArea(ctx context.Context, a *model.ParkingArea) error
	ArchiveParkingArea(ctx context.Context, id string, now time.Time) error
	ListParkingAreas(ctx context.Context, page model.Page) ([]*model.ParkingArea, int64, error)
	CreateAuthorization(ctx context.Context, a *model.Authorization, now time.Time) error
	GetAuthorization(ctx context.Context, id string) (*model.Authorization, error)
	UpdateAuthorization(ctx context.Context, a *model.Authorization) error // optimistic lock
	ArchiveAuthorization(ctx context.Context, id string, now time.Time) error
	ListAuthorizations(ctx context.Context, f model.AuthFilter) ([]*model.Authorization, int64, error)
	CreateRecord(ctx context.Context, r *model.EntryExitRecord) error
	GetRecord(ctx context.Context, id string) (*model.EntryExitRecord, error)
	ListRecords(ctx context.Context, f RecordFilter) ([]*model.EntryExitRecord, int64, error)
	EnterVehicle(ctx context.Context, authID string, now time.Time) (*model.EntryExitRecord, error)
	ExitVehicle(ctx context.Context, authID string, now time.Time, operator, note string) (*model.EntryExitRecord, error)
	RevokeAuthorization(ctx context.Context, authID string, now time.Time, operator, reason string) (*model.Authorization, error)
	CreateAuditLog(ctx context.Context, l *model.AuditLog) error
	ListAuditLogs(ctx context.Context, entityType string, page model.Page) ([]*model.AuditLog, int64, error)
	ListCurrentVehicles(ctx context.Context, areaID string, page model.Page) ([]*model.EntryExitRecord, int64, error)
	AreaOccupancy(ctx context.Context) ([]*model.AreaOccupancy, error)
}
