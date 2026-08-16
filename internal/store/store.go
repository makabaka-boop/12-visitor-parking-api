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
	ErrAlreadySettled   = errors.New("fee already settled")
)

type RecordFilter struct {
	AreaID string
	Status string // entered | exited
	Plate  string
	Page   model.Page
}
type BillingRuleFilter struct {
	ParkingAreaID string
	Status        string // active | archived
	Page          model.Page
}
type FeeFilter struct {
	AreaID   string
	Plate    string
	RecordID string
	Status   string // unsettled | settled
	Page     model.Page
}
type AreaRevenueFilter struct {
	AreaID string
	From   *time.Time // exit_time >= From
	To     *time.Time // exit_time < To
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
	ExitVehicle(ctx context.Context, authID string, now time.Time, operator, note string) (*model.EntryExitRecord, *model.Fee, error)
	RevokeAuthorization(ctx context.Context, authID string, now time.Time, operator, reason string) (*model.Authorization, error)
	CreateAuditLog(ctx context.Context, l *model.AuditLog) error
	ListAuditLogs(ctx context.Context, entityType string, page model.Page) ([]*model.AuditLog, int64, error)
	ListCurrentVehicles(ctx context.Context, areaID string, page model.Page) ([]*model.EntryExitRecord, int64, error)
	AreaOccupancy(ctx context.Context) ([]*model.AreaOccupancy, error)
	CreateBillingRule(ctx context.Context, r *model.BillingRule) error
	GetBillingRule(ctx context.Context, id string) (*model.BillingRule, error)
	UpdateBillingRule(ctx context.Context, r *model.BillingRule, expectedUpdatedAt time.Time) error
	ArchiveBillingRule(ctx context.Context, id string, now time.Time) error
	ListBillingRules(ctx context.Context, f BillingRuleFilter) ([]*model.BillingRule, int64, error)
	// ActiveBillingRule returns the rule effective for areaID at time at. Rules
	// archived after at remain eligible because they were active at that time.
	ActiveBillingRule(ctx context.Context, areaID string, at time.Time) (*model.BillingRule, error)
	CreateFee(ctx context.Context, f *model.Fee) error
	GetFee(ctx context.Context, id string) (*model.Fee, error)
	ListFees(ctx context.Context, f FeeFilter) ([]*model.Fee, int64, error)
	// SettleFee atomically marks an unsettled fee as settled. A fee that is
	// already settled yields ErrAlreadySettled. method/reason/operator are
	// stored verbatim; validation is the caller's responsibility.
	SettleFee(ctx context.Context, id, method, reason, operator string, now time.Time) (*model.Fee, error)
	AreaRevenue(ctx context.Context, f AreaRevenueFilter) ([]*model.AreaRevenue, error)
}
