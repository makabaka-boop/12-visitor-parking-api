package model
import "time"
// MaxAuthDuration caps any single authorization's total validity window.
// Shared by service (validation) and store (defensive re-checks).
const MaxAuthDuration = 7 * 24 * time.Hour
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
// Extension application statuses.
const (
	ExtStatusPending  = "pending"  // 待审批
	ExtStatusApproved = "approved" // 已通过
	ExtStatusRejected = "rejected" // 已驳回
	ExtStatusRevoked  = "revoked"  // 已撤销
)
// ExtensibleAuth reports whether an authorization may still be extended.
// Both pending and active authorizations qualify only while their validity
// window has not yet elapsed (now is before endTime): once the end time has
// been reached the authorization is effectively expired and may not be
// extended, regardless of its stored status. (Active status is not derived to
// "expired", so the time check is required here; the vehicle may still be on
// the premises, but its authorized window has passed.) Completed, cancelled
// and expired authorizations are not extensible.
func ExtensibleAuth(status string, endTime time.Time, now time.Time) bool {
	if status != AuthStatusPending && status != AuthStatusActive {
		return false
	}
	// An authorization whose validity window has passed is expired and not
	// extensible, whether its stored status is pending or active.
	if !now.Before(endTime) {
		return false
	}
	return true
}
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
// ExtensionApplication is a property-staff request to push an authorization's
// end_time later. Only one pending application may exist per authorization.
type ExtensionApplication struct {
	ID              string     `json:"id"`
	AuthorizationID string     `json:"authorization_id"`
	Plate           string     `json:"plate"`
	OriginalEndTime time.Time  `json:"original_end_time"`
	NewEndTime      time.Time  `json:"new_end_time"`
	Reason          string     `json:"reason"`        // 延期原因（申请人填写）
	Applicant       string     `json:"applicant"`     // 申请人
	Status          string     `json:"status"`        // pending|approved|rejected|revoked
	DecidedBy       string     `json:"decided_by,omitempty"`     // 审批人/驳回人/撤销人
	DecidedAt       *time.Time `json:"decided_at,omitempty"`     // 审批/决策时间
	DecisionNote    string     `json:"decision_note,omitempty"`  // 审批备注/驳回原因/撤销原因
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"` // also used as optimistic-lock version token
}
// ExtensionAppFilter scopes the paginated extension-application query.
type ExtensionAppFilter struct {
	AuthorizationID string
	Plate           string
	Status          string
	Applicant       string
	Page            Page
}
