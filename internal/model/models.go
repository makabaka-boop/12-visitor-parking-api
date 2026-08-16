package model
import "time"
const (
	AuthStatusPending   = "pending"   // created, not yet entered
	AuthStatusActive    = "active"    // vehicle has entered, occupying a spot
	AuthStatusCompleted = "completed" // vehicle has exited
	AuthStatusCancelled = "cancelled" // revoked by property staff before entry
	AuthStatusExpired   = "expired"   // validity window passed without entry
)
const (
	RecordStatusEntered = "entered"
	RecordStatusExited  = "exited"
)
const (
	ResidentActive   = "active"
	ResidentDisabled = "disabled"
)
type Page struct {
	Limit  int
	Offset int
}
type Paginated[T any] struct {
	Items []T   `json:"items"`
	Total int64 `json:"total"`
	Page  int   `json:"page"`
	Size  int   `json:"size"`
}
type Resident struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Phone      string     `json:"phone"`
	Building   string     `json:"building"` // 楼栋
	Unit       string     `json:"unit"`
	Room       string     `json:"room"`
	Status     string     `json:"status"` // active | disabled
	ArchivedAt *time.Time `json:"archived_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}
type Vehicle struct {
	ID         string     `json:"id"`
	Plate      string     `json:"plate"`
	OwnerName  string     `json:"owner_name"`
	OwnerPhone string     `json:"owner_phone"`
	Color      string     `json:"color"`
	ArchivedAt *time.Time `json:"archived_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}
type ParkingArea struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Code       string     `json:"code"`
	Capacity   int        `json:"capacity"`
	ArchivedAt *time.Time `json:"archived_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}
type Authorization struct {
	ID            string     `json:"id"`
	ResidentID    string     `json:"resident_id"`
	Plate         string     `json:"plate"`
	ParkingAreaID string     `json:"parking_area_id"`
	StartTime     time.Time  `json:"start_time"`
	EndTime       time.Time  `json:"end_time"`
	Status        string     `json:"status"`
	Purpose       string     `json:"purpose"`
	CreatedBy     string     `json:"created_by"`
	ArchivedAt    *time.Time `json:"archived_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"` // also used as optimistic-lock version token
}
type EntryExitRecord struct {
	ID              string     `json:"id"`
	AuthorizationID string     `json:"authorization_id"`
	Plate           string     `json:"plate"`
	ParkingAreaID   string     `json:"parking_area_id"`
	EntryTime       time.Time  `json:"entry_time"`
	ExitTime        *time.Time `json:"exit_time,omitempty"`
	ExitOperator    string     `json:"exit_operator,omitempty"`
	ExitNote        string     `json:"exit_note,omitempty"`
	Status          string     `json:"status"` // entered | exited
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}
type AuditLog struct {
	ID         string    `json:"id"`
	Action     string    `json:"action"`      // e.g. authorization.create, entry.record
	EntityType string    `json:"entity_type"` // resident | authorization | record ...
	EntityID   string    `json:"entity_id"`
	Operator   string    `json:"operator"`
	Detail     string    `json:"detail"`
	CreatedAt  time.Time `json:"created_at"`
}
type AuthFilter struct {
	Building      string // join through resident
	Plate         string // exact plate match
	ParkingAreaID string
	Status        string
	ValidOn       *time.Time // authorizations whose window covers this moment
	ValidOnNow    *time.Time // "now" used to derive the effective status (expired)
	StartFrom     *time.Time // start_time > StartFrom (today's arrivals lower bound)
	StartTo       *time.Time // start_time < StartTo (today's arrivals upper bound)
	EndingBefore  *time.Time // end_time < EndingBefore (expiring-soon cutoff)
	Page          Page
}
type AreaOccupancy struct {
	AreaID      string  `json:"area_id"`
	AreaName    string  `json:"area_name"`
	Code        string  `json:"code"`
	Capacity    int     `json:"capacity"`
	Occupied    int     `json:"occupied"`
	Available   int     `json:"available"`
	Utilization float64 `json:"utilization"` // occupied / capacity
}
const (
	RestrictionTypeForbidden     = "forbidden"       // 禁止入场：直接拒绝
	RestrictionTypeManualConfirm = "manual_confirm"  // 人工确认：请求须提交确认人
)
const (
	RestrictionStatusActive   = "active"   // 生效中
	RestrictionStatusReleased = "released" // 已解除（历史保留，不可恢复为 active）
)
// VehicleRestriction is a vehicle plate on the restriction list managed by
// property staff. A plate may be forbidden from entry or require manual
// confirmation during a given [EffectiveFrom, EffectiveTo) window.
type VehicleRestriction struct {
	ID            string     `json:"id"`
	Plate         string     `json:"plate"`
	Type          string     `json:"type"` // forbidden | manual_confirm
	EffectiveFrom time.Time  `json:"effective_from"`
	EffectiveTo   time.Time  `json:"effective_to"`
	Reason        string     `json:"reason"`
	RegisteredBy  string     `json:"registered_by"`
	Status        string     `json:"status"` // active | released
	ArchivedAt    *time.Time `json:"archived_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"` // also used as optimistic-lock version token
}
// RestrictionFilter is the combined filter for listing vehicle restrictions.
type RestrictionFilter struct {
	Plate        string
	Type         string // forbidden | manual_confirm
	Status       string // active | released
	RegisteredBy string
	EffectiveOn  *time.Time // restrictions in effect at this instant
	Page         Page
}
// RestrictionStats summarises the current restriction list.
type RestrictionStats struct {
	TotalActive       int64 `json:"total_active"`        // status=active, not archived
	CurrentlyInEffect int64 `json:"currently_in_effect"` // active AND now within [effective_from, effective_to)
	Forbidden         int64 `json:"forbidden"`           // active, type=forbidden
	ManualConfirm     int64 `json:"manual_confirm"`      // active, type=manual_confirm
	Released          int64 `json:"released"`            // status=released (history retained)
}
