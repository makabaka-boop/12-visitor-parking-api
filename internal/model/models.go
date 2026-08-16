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
const (
	BillingRuleActive   = "active"
	BillingRuleArchived = "archived"
)
const (
	FeeStatusUnsettled = "unsettled"
	FeeStatusSettled   = "settled"
)
const (
	SettleCash   = "cash"
	SettleOnline = "online"
	SettleWaiver = "waiver"
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

// BillingRule is the temporary-parking tariff configured per parking area.
// All monetary amounts are stored as integer cents (1元 = 100分) to avoid
// floating-point rounding errors.
type BillingRule struct {
	ID              string     `json:"id"`
	ParkingAreaID   string     `json:"parking_area_id"`
	FreeMinutes     int        `json:"free_minutes"`      // 免费分钟数
	HourlyRateCents int64      `json:"hourly_rate_cents"` // 小时单价 (分)
	DailyCapCents   int64      `json:"daily_cap_cents"`   // 每日封顶金额 (分), 0 = 不封顶
	EffectiveFrom   time.Time  `json:"effective_from"`    // 启用时间
	Status          string     `json:"status"`            // active | archived
	ArchivedAt      *time.Time `json:"archived_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// Fee is the charge generated when a vehicle exits. It is self-contained:
// once created the amount never changes, even if the originating rule is
// later modified or archived.
type Fee struct {
	ID              string     `json:"id"`
	RecordID        string     `json:"record_id"`
	AuthorizationID string     `json:"authorization_id"`
	Plate           string     `json:"plate"`
	ParkingAreaID   string     `json:"parking_area_id"`
	BillingRuleID   string     `json:"billing_rule_id"`
	EntryTime       time.Time  `json:"entry_time"`
	ExitTime        time.Time  `json:"exit_time"`
	DurationMinutes int64      `json:"duration_minutes"`        // 实际停留分钟
	ChargedMinutes  int64      `json:"charged_minutes"`         // 计费分钟 (扣除免费后, 按小时向上取整)
	AmountCents     int64      `json:"amount_cents"`            // 应收金额 (分)
	Status          string     `json:"status"`                  // unsettled | settled
	SettleMethod    string     `json:"settle_method,omitempty"` // cash | online | waiver
	SettleReason    string     `json:"settle_reason,omitempty"` // 减免原因 (减免必填)
	SettleOperator  string     `json:"settle_operator,omitempty"`
	SettledAt       *time.Time `json:"settled_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// AreaRevenue summarises collected and outstanding fees per parking area.
type AreaRevenue struct {
	AreaID         string `json:"area_id"`
	AreaName       string `json:"area_name"`
	Code           string `json:"code"`
	FeeCount       int64  `json:"fee_count"`
	SettledCount   int64  `json:"settled_count"`
	UnsettledCount int64  `json:"unsettled_count"`
	SettledCents   int64  `json:"settled_cents"`
	UnsettledCents int64  `json:"unsettled_cents"`
	TotalCents     int64  `json:"total_cents"`
}
