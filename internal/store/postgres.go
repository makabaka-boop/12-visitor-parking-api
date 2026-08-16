package store
import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	_ "github.com/lib/pq"
	"visitor-parking/internal/model"
)
type Postgres struct{ db *sql.DB }
func NewPostgres(dsn string) (*Postgres, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	return &Postgres{db: db}, nil
}
func (p *Postgres) Close() error { return p.db.Close() }
func (p *Postgres) DB() *sql.DB  { return p.db }
type txKey struct{}
type scanner interface {
	Scan(dest ...interface{}) error
}
type querier interface {
	ExecContext(ctx context.Context, q string, args ...interface{}) (sql.Result, error)
	QueryRowContext(ctx context.Context, q string, args ...interface{}) *sql.Row
	QueryContext(ctx context.Context, q string, args ...interface{}) (*sql.Rows, error)
}
func (p *Postgres) q(ctx context.Context) querier {
	if v, ok := ctx.Value(txKey{}).(querier); ok {
		return v
	}
	return p.db
}
func (p *Postgres) runTx(ctx context.Context, fn func(ctx context.Context) error) error {
	tx, err := p.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	if err := fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
func mapNotFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
func checkRows(res sql.Result, fail error) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fail
	}
	return nil
}
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "duplicate key") || strings.Contains(s, "unique constraint")
}
func scanResident(s scanner, r *model.Resident) error {
	return s.Scan(&r.ID, &r.Name, &r.Phone, &r.Building, &r.Unit, &r.Room, &r.Status, &r.ArchivedAt, &r.CreatedAt, &r.UpdatedAt)
}
func scanVehicle(s scanner, v *model.Vehicle) error {
	return s.Scan(&v.ID, &v.Plate, &v.OwnerName, &v.OwnerPhone, &v.Color, &v.ArchivedAt, &v.CreatedAt, &v.UpdatedAt)
}
func scanArea(s scanner, a *model.ParkingArea) error {
	return s.Scan(&a.ID, &a.Name, &a.Code, &a.Capacity, &a.ArchivedAt, &a.CreatedAt, &a.UpdatedAt)
}
func scanAuth(s scanner, a *model.Authorization) error {
	return s.Scan(&a.ID, &a.ResidentID, &a.Plate, &a.ParkingAreaID, &a.StartTime, &a.EndTime, &a.Status, &a.Purpose, &a.CreatedBy, &a.ArchivedAt, &a.CreatedAt, &a.UpdatedAt)
}
func scanRecord(s scanner, r *model.EntryExitRecord) error {
	return s.Scan(&r.ID, &r.AuthorizationID, &r.Plate, &r.ParkingAreaID, &r.EntryTime, &r.ExitTime, &r.ExitOperator, &r.ExitNote, &r.Status, &r.CreatedAt, &r.UpdatedAt)
}
const residentCols = "id,name,phone,building,unit,room,status,archived_at,created_at,updated_at"
func (p *Postgres) CreateResident(ctx context.Context, r *model.Resident) error {
	_, err := p.q(ctx).ExecContext(ctx, `INSERT INTO residents (`+residentCols+`) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		r.ID, r.Name, r.Phone, r.Building, r.Unit, r.Room, r.Status, r.ArchivedAt, r.CreatedAt, r.UpdatedAt)
	if err != nil && isUniqueViolation(err) {
		return ErrConflict
	}
	return err
}
func (p *Postgres) GetResident(ctx context.Context, id string) (*model.Resident, error) {
	r := &model.Resident{}
	err := scanResident(p.q(ctx).QueryRowContext(ctx, `SELECT `+residentCols+` FROM residents WHERE id=$1 AND archived_at IS NULL`, id), r)
	return r, mapNotFound(err)
}
func (p *Postgres) UpdateResident(ctx context.Context, r *model.Resident) error {
	res, err := p.q(ctx).ExecContext(ctx, `UPDATE residents SET name=$1,phone=$2,building=$3,unit=$4,room=$5,status=$6,updated_at=$7 WHERE id=$8 AND archived_at IS NULL AND updated_at=$9`,
		r.Name, r.Phone, r.Building, r.Unit, r.Room, r.Status, r.UpdatedAt, r.ID, r.UpdatedAt)
	return wrap(res, err, ErrConcurrentModify)
}
func (p *Postgres) ArchiveResident(ctx context.Context, id string, now time.Time) error {
	res, err := p.q(ctx).ExecContext(ctx, `UPDATE residents SET archived_at=$1,updated_at=$2 WHERE id=$3 AND archived_at IS NULL`, now, now, id)
	return wrap(res, err, ErrNotFound)
}
func (p *Postgres) ListResidents(ctx context.Context, page model.Page) ([]*model.Resident, int64, error) {
	page = normPage(page)
	q := `SELECT count(*) OVER(),` + residentCols + ` FROM residents WHERE archived_at IS NULL ORDER BY created_at DESC LIMIT $1 OFFSET $2`
	rows, err := p.db.QueryContext(ctx, q, page.Limit, page.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*model.Resident
	var total int64
	for rows.Next() {
		r := &model.Resident{}
		if err := rows.Scan(&total, &r.ID, &r.Name, &r.Phone, &r.Building, &r.Unit, &r.Room, &r.Status, &r.ArchivedAt, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	return out, total, rows.Err()
}
func wrap(res sql.Result, err error, fail error) error {
	if err != nil {
		return err
	}
	return checkRows(res, fail)
}
const vehicleCols = "id,plate,owner_name,owner_phone,color,archived_at,created_at,updated_at"
func (p *Postgres) CreateVehicle(ctx context.Context, v *model.Vehicle) error {
	_, err := p.q(ctx).ExecContext(ctx, `INSERT INTO vehicles (`+vehicleCols+`) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		v.ID, v.Plate, v.OwnerName, v.OwnerPhone, v.Color, v.ArchivedAt, v.CreatedAt, v.UpdatedAt)
	if err != nil && isUniqueViolation(err) {
		return ErrConflict
	}
	return err
}
func (p *Postgres) GetVehicle(ctx context.Context, id string) (*model.Vehicle, error) {
	v := &model.Vehicle{}
	err := scanVehicle(p.q(ctx).QueryRowContext(ctx, `SELECT `+vehicleCols+` FROM vehicles WHERE id=$1 AND archived_at IS NULL`, id), v)
	return v, mapNotFound(err)
}
func (p *Postgres) GetVehicleByPlate(ctx context.Context, plate string) (*model.Vehicle, error) {
	v := &model.Vehicle{}
	err := scanVehicle(p.q(ctx).QueryRowContext(ctx, `SELECT `+vehicleCols+` FROM vehicles WHERE plate=$1 AND archived_at IS NULL`, plate), v)
	return v, mapNotFound(err)
}
func (p *Postgres) UpdateVehicle(ctx context.Context, v *model.Vehicle) error {
	res, err := p.q(ctx).ExecContext(ctx, `UPDATE vehicles SET owner_name=$1,owner_phone=$2,color=$3,plate=$4,updated_at=$5 WHERE id=$6 AND archived_at IS NULL AND updated_at=$7`,
		v.OwnerName, v.OwnerPhone, v.Color, v.Plate, v.UpdatedAt, v.ID, v.UpdatedAt)
	return wrap(res, err, ErrConcurrentModify)
}
func (p *Postgres) ArchiveVehicle(ctx context.Context, id string, now time.Time) error {
	res, err := p.q(ctx).ExecContext(ctx, `UPDATE vehicles SET archived_at=$1,updated_at=$2 WHERE id=$3 AND archived_at IS NULL`, now, now, id)
	return wrap(res, err, ErrNotFound)
}
func (p *Postgres) ListVehicles(ctx context.Context, page model.Page) ([]*model.Vehicle, int64, error) {
	page = normPage(page)
	rows, err := p.db.QueryContext(ctx, `SELECT count(*) OVER(),`+vehicleCols+` FROM vehicles WHERE archived_at IS NULL ORDER BY created_at DESC LIMIT $1 OFFSET $2`, page.Limit, page.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*model.Vehicle
	var total int64
	for rows.Next() {
		v := &model.Vehicle{}
		if err := rows.Scan(&total, &v.ID, &v.Plate, &v.OwnerName, &v.OwnerPhone, &v.Color, &v.ArchivedAt, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, v)
	}
	return out, total, rows.Err()
}
const areaCols = "id,name,code,capacity,archived_at,created_at,updated_at"
func (p *Postgres) CreateParkingArea(ctx context.Context, a *model.ParkingArea) error {
	_, err := p.q(ctx).ExecContext(ctx, `INSERT INTO parking_areas (`+areaCols+`) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		a.ID, a.Name, a.Code, a.Capacity, a.ArchivedAt, a.CreatedAt, a.UpdatedAt)
	if err != nil && isUniqueViolation(err) {
		return ErrConflict
	}
	return err
}
func (p *Postgres) GetParkingArea(ctx context.Context, id string) (*model.ParkingArea, error) {
	a := &model.ParkingArea{}
	err := scanArea(p.q(ctx).QueryRowContext(ctx, `SELECT `+areaCols+` FROM parking_areas WHERE id=$1 AND archived_at IS NULL`, id), a)
	return a, mapNotFound(err)
}
func (p *Postgres) UpdateParkingArea(ctx context.Context, a *model.ParkingArea) error {
	res, err := p.q(ctx).ExecContext(ctx, `UPDATE parking_areas SET name=$1,code=$2,capacity=$3,updated_at=$4 WHERE id=$5 AND archived_at IS NULL AND updated_at=$6`,
		a.Name, a.Code, a.Capacity, a.UpdatedAt, a.ID, a.UpdatedAt)
	return wrap(res, err, ErrConcurrentModify)
}
func (p *Postgres) ArchiveParkingArea(ctx context.Context, id string, now time.Time) error {
	res, err := p.q(ctx).ExecContext(ctx, `UPDATE parking_areas SET archived_at=$1,updated_at=$2 WHERE id=$3 AND archived_at IS NULL`, now, now, id)
	return wrap(res, err, ErrNotFound)
}
func (p *Postgres) ListParkingAreas(ctx context.Context, page model.Page) ([]*model.ParkingArea, int64, error) {
	page = normPage(page)
	rows, err := p.db.QueryContext(ctx, `SELECT count(*) OVER(),`+areaCols+` FROM parking_areas WHERE archived_at IS NULL ORDER BY created_at DESC LIMIT $1 OFFSET $2`, page.Limit, page.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*model.ParkingArea
	var total int64
	for rows.Next() {
		a := &model.ParkingArea{}
		if err := rows.Scan(&total, &a.ID, &a.Name, &a.Code, &a.Capacity, &a.ArchivedAt, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, a)
	}
	return out, total, rows.Err()
}
const authCols = "id,resident_id,plate,parking_area_id,start_time,end_time,status,purpose,created_by,archived_at,created_at,updated_at"
func (p *Postgres) CreateAuthorization(ctx context.Context, a *model.Authorization, now time.Time) error {
	return p.runTx(ctx, func(ctx context.Context) error {
		var status string
		if err := p.q(ctx).QueryRowContext(ctx, `SELECT status FROM residents WHERE id=$1 AND archived_at IS NULL`, a.ResidentID).Scan(&status); err != nil {
			return mapNotFound(err)
		}
		if status != model.ResidentActive {
			return ErrResidentDisabled
		}
		var archived *time.Time
		if err := p.q(ctx).QueryRowContext(ctx, `SELECT archived_at FROM parking_areas WHERE id=$1`, a.ParkingAreaID).Scan(&archived); err != nil {
			return mapNotFound(err)
		}
		if archived != nil {
			return ErrAreaArchived
		}
		var n int64
		if err := p.q(ctx).QueryRowContext(ctx, `SELECT count(*) FROM authorizations WHERE plate=$1 AND archived_at IS NULL AND status IN ('pending','active') AND start_time < $2 AND end_time > $3`, a.Plate, a.EndTime, a.StartTime).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return ErrConflict
		}
		_, err := p.q(ctx).ExecContext(ctx, `INSERT INTO authorizations (`+authCols+`) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
			a.ID, a.ResidentID, a.Plate, a.ParkingAreaID, a.StartTime, a.EndTime, a.Status, a.Purpose, a.CreatedBy, a.ArchivedAt, a.CreatedAt, a.UpdatedAt)
		return err
	})
}
func (p *Postgres) GetAuthorization(ctx context.Context, id string) (*model.Authorization, error) {
	a := &model.Authorization{}
	err := scanAuth(p.q(ctx).QueryRowContext(ctx, `SELECT `+authCols+` FROM authorizations WHERE id=$1 AND archived_at IS NULL`, id), a)
	return a, mapNotFound(err)
}
func (p *Postgres) UpdateAuthorization(ctx context.Context, a *model.Authorization) error {
	res, err := p.q(ctx).ExecContext(ctx, `UPDATE authorizations SET start_time=$1,end_time=$2,status=$3,purpose=$4,parking_area_id=$5,updated_at=$6 WHERE id=$7 AND archived_at IS NULL AND updated_at=$8`,
		a.StartTime, a.EndTime, a.Status, a.Purpose, a.ParkingAreaID, a.UpdatedAt, a.ID, a.UpdatedAt)
	return wrap(res, err, ErrConcurrentModify)
}
func (p *Postgres) ArchiveAuthorization(ctx context.Context, id string, now time.Time) error {
	res, err := p.q(ctx).ExecContext(ctx, `UPDATE authorizations SET archived_at=$1,updated_at=$2 WHERE id=$3 AND archived_at IS NULL`, now, now, id)
	return wrap(res, err, ErrNotFound)
}
func (p *Postgres) ListAuthorizations(ctx context.Context, f model.AuthFilter) ([]*model.Authorization, int64, error) {
	var sb strings.Builder
	args := []interface{}{}
	sb.WriteString(`SELECT count(*) OVER(),a.` + strings.Join(strings.Split(authCols, ","), ",a.") +
		` FROM authorizations a LEFT JOIN residents r ON r.id=a.resident_id WHERE a.archived_at IS NULL`)
	n := 1
	add := func(cond string, arg interface{}) {
		sb.WriteString(fmt.Sprintf(cond, n))
		args = append(args, arg)
		n++
	}
	if f.Building != "" {
		add(" AND r.building=$%d", f.Building)
	}
	if f.Plate != "" {
		add(" AND a.plate=$%d", f.Plate)
	}
	if f.ParkingAreaID != "" {
		add(" AND a.parking_area_id=$%d", f.ParkingAreaID)
	}
	if f.Status != "" {
		if f.Status == model.AuthStatusExpired && f.ValidOnNow != nil {
			add(" AND a.status='pending' AND a.end_time <= $%d", *f.ValidOnNow)
		} else {
			add(" AND a.status=$%d", f.Status)
		}
	} else if f.ValidOn != nil {
		add(" AND a.start_time < $%d AND a.end_time > $%d", *f.ValidOn)
		n++ // same arg used for both placeholders
	}
	if f.StartFrom != nil {
		add(" AND a.start_time > $%d", *f.StartFrom)
	}
	if f.StartTo != nil {
		add(" AND a.start_time < $%d", *f.StartTo)
	}
	if f.EndingBefore != nil {
		add(" AND a.end_time < $%d AND a.status='pending'", *f.EndingBefore)
	}
	sb.WriteString(" ORDER BY a.start_time DESC")
	page := normPage(f.Page)
	sb.WriteString(fmt.Sprintf(" LIMIT $%d OFFSET $%d", n, n+1))
	args = append(args, page.Limit, page.Offset)
	rows, err := p.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*model.Authorization
	var total int64
	for rows.Next() {
		a := &model.Authorization{}
		if err := rows.Scan(&total, &a.ID, &a.ResidentID, &a.Plate, &a.ParkingAreaID, &a.StartTime, &a.EndTime, &a.Status, &a.Purpose, &a.CreatedBy, &a.ArchivedAt, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, a)
	}
	return out, total, rows.Err()
}
const recordCols = "id,authorization_id,plate,parking_area_id,entry_time,exit_time,exit_operator,exit_note,status,created_at,updated_at"
func (p *Postgres) CreateRecord(ctx context.Context, r *model.EntryExitRecord) error {
	_, err := p.q(ctx).ExecContext(ctx, `INSERT INTO entry_exit_records (`+recordCols+`) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		r.ID, r.AuthorizationID, r.Plate, r.ParkingAreaID, r.EntryTime, r.ExitTime, r.ExitOperator, r.ExitNote, r.Status, r.CreatedAt, r.UpdatedAt)
	return err
}
func (p *Postgres) GetRecord(ctx context.Context, id string) (*model.EntryExitRecord, error) {
	r := &model.EntryExitRecord{}
	err := scanRecord(p.q(ctx).QueryRowContext(ctx, `SELECT `+recordCols+` FROM entry_exit_records WHERE id=$1`, id), r)
	return r, mapNotFound(err)
}
func (p *Postgres) ListRecords(ctx context.Context, f RecordFilter) ([]*model.EntryExitRecord, int64, error) {
	var sb strings.Builder
	args := []interface{}{}
	sb.WriteString(`SELECT count(*) OVER(),` + recordCols + ` FROM entry_exit_records WHERE 1=1`)
	n := 1
	add := func(cond string, arg interface{}) {
		sb.WriteString(fmt.Sprintf(cond, n))
		args = append(args, arg)
		n++
	}
	if f.AreaID != "" {
		add(" AND parking_area_id=$%d", f.AreaID)
	}
	if f.Status != "" {
		add(" AND status=$%d", f.Status)
	}
	if f.Plate != "" {
		add(" AND plate=$%d", f.Plate)
	}
	sb.WriteString(" ORDER BY entry_time DESC")
	page := normPage(f.Page)
	sb.WriteString(fmt.Sprintf(" LIMIT $%d OFFSET $%d", n, n+1))
	args = append(args, page.Limit, page.Offset)
	rows, err := p.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*model.EntryExitRecord
	var total int64
	for rows.Next() {
		r := &model.EntryExitRecord{}
		if err := rows.Scan(&total, &r.ID, &r.AuthorizationID, &r.Plate, &r.ParkingAreaID, &r.EntryTime, &r.ExitTime, &r.ExitOperator, &r.ExitNote, &r.Status, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	return out, total, rows.Err()
}
func (p *Postgres) CreateAuditLog(ctx context.Context, l *model.AuditLog) error {
	_, err := p.q(ctx).ExecContext(ctx, `INSERT INTO audit_logs (id,action,entity_type,entity_id,operator,detail,created_at) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		l.ID, l.Action, l.EntityType, l.EntityID, l.Operator, l.Detail, l.CreatedAt)
	return err
}
func (p *Postgres) ListAuditLogs(ctx context.Context, entityType string, page model.Page) ([]*model.AuditLog, int64, error) {
	page = normPage(page)
	q := `SELECT count(*) OVER(),id,action,entity_type,entity_id,operator,detail,created_at FROM audit_logs`
	args := []interface{}{}
	if entityType != "" {
		q += " WHERE entity_type=$1"
		args = append(args, entityType)
	}
	q += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	args = append(args, page.Limit, page.Offset)
	rows, err := p.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*model.AuditLog
	var total int64
	for rows.Next() {
		l := &model.AuditLog{}
		if err := rows.Scan(&total, &l.ID, &l.Action, &l.EntityType, &l.EntityID, &l.Operator, &l.Detail, &l.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, l)
	}
	return out, total, rows.Err()
}
func (p *Postgres) ListCurrentVehicles(ctx context.Context, areaID string, page model.Page) ([]*model.EntryExitRecord, int64, error) {
	return p.ListRecords(ctx, RecordFilter{AreaID: areaID, Status: model.RecordStatusEntered, Page: page})
}
func (p *Postgres) AreaOccupancy(ctx context.Context) ([]*model.AreaOccupancy, error) {
	q := `SELECT a.id,a.name,a.code,a.capacity,COALESCE((SELECT count(*) FROM entry_exit_records r WHERE r.parking_area_id=a.id AND r.status='entered'),0) FROM parking_areas a WHERE a.archived_at IS NULL ORDER BY occupied DESC`
	rows, err := p.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.AreaOccupancy
	for rows.Next() {
		o := &model.AreaOccupancy{}
		if err := rows.Scan(&o.AreaID, &o.AreaName, &o.Code, &o.Capacity, &o.Occupied); err != nil {
			return nil, err
		}
		if o.Available = o.Capacity - o.Occupied; o.Available < 0 {
			o.Available = 0
		}
		if o.Capacity > 0 {
			o.Utilization = float64(o.Occupied) / float64(o.Capacity)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
func (p *Postgres) EnterVehicle(ctx context.Context, authID string, now time.Time) (*model.EntryExitRecord, error) {
	var rec *model.EntryExitRecord
	err := p.runTx(ctx, func(ctx context.Context) error {
		var plate, areaID string
		var start, end time.Time
		var status string
		err := p.q(ctx).QueryRowContext(ctx, `SELECT plate,parking_area_id,start_time,end_time,status FROM authorizations WHERE id=$1 AND archived_at IS NULL FOR UPDATE`, authID).
			Scan(&plate, &areaID, &start, &end, &status)
		if err != nil {
			return mapNotFound(err)
		}
		if status != model.AuthStatusPending {
			if status == model.AuthStatusActive {
				return ErrAlreadyEntered
			}
			return ErrStatusTransition
		}
		if now.Before(start) || !now.Before(end) {
			return ErrOutOfTimeWindow
		}
		var capacity, occupied int
		if err := p.q(ctx).QueryRowContext(ctx, `SELECT capacity,COALESCE((SELECT count(*) FROM entry_exit_records r WHERE r.parking_area_id=p.id AND r.status='entered'),0) FROM parking_areas p WHERE id=$1 AND archived_at IS NULL FOR UPDATE`, areaID).Scan(&capacity, &occupied); err != nil {
			return mapNotFound(err)
		}
		if occupied >= capacity {
			return ErrNoCapacity
		}
		rec = &model.EntryExitRecord{ID: NewID("rec"), AuthorizationID: authID, Plate: plate, ParkingAreaID: areaID, EntryTime: now, Status: model.RecordStatusEntered, CreatedAt: now, UpdatedAt: now}
		if _, err := p.q(ctx).ExecContext(ctx, `INSERT INTO entry_exit_records (`+recordCols+`) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, rec.ID, rec.AuthorizationID, rec.Plate, rec.ParkingAreaID, rec.EntryTime, rec.ExitTime, rec.ExitOperator, rec.ExitNote, rec.Status, rec.CreatedAt, rec.UpdatedAt); err != nil {
			return err
		}
		_, err = p.q(ctx).ExecContext(ctx, `UPDATE authorizations SET status='active',updated_at=$1 WHERE id=$2`, now, authID)
		return err
	})
	return rec, err
}
func (p *Postgres) ExitVehicle(ctx context.Context, authID string, now time.Time, operator, note string) (*model.EntryExitRecord, error) {
	var recID string
	err := p.runTx(ctx, func(ctx context.Context) error {
		var status string
		if err := p.q(ctx).QueryRowContext(ctx, `SELECT status FROM authorizations WHERE id=$1 AND archived_at IS NULL FOR UPDATE`, authID).Scan(&status); err != nil {
			return mapNotFound(err)
		}
		if status == model.AuthStatusCompleted {
			return ErrAlreadyExited
		}
		if status != model.AuthStatusActive {
			return ErrStatusTransition
		}
		if err := p.q(ctx).QueryRowContext(ctx, `SELECT id FROM entry_exit_records WHERE authorization_id=$1 AND status='entered' FOR UPDATE`, authID).Scan(&recID); err != nil {
			return mapNotFound(err)
		}
		if _, err := p.q(ctx).ExecContext(ctx, `UPDATE entry_exit_records SET exit_time=$1,exit_operator=$2,exit_note=$3,status='exited',updated_at=$4 WHERE id=$5`, now, operator, note, now, recID); err != nil {
			return err
		}
		_, err := p.q(ctx).ExecContext(ctx, `UPDATE authorizations SET status='completed',updated_at=$1 WHERE id=$2`, now, authID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return p.GetRecord(ctx, recID)
}
func (p *Postgres) RevokeAuthorization(ctx context.Context, authID string, now time.Time, operator, reason string) (*model.Authorization, error) {
	err := p.runTx(ctx, func(ctx context.Context) error {
		var status string
		if err := p.q(ctx).QueryRowContext(ctx, `SELECT status FROM authorizations WHERE id=$1 AND archived_at IS NULL FOR UPDATE`, authID).Scan(&status); err != nil {
			return mapNotFound(err)
		}
		if status == model.AuthStatusActive {
			return fmt.Errorf("%w: active authorization must be exited first", ErrStatusTransition)
		}
		if status != model.AuthStatusPending {
			return ErrStatusTransition
		}
		_, err := p.q(ctx).ExecContext(ctx, `UPDATE authorizations SET status='cancelled',updated_at=$1 WHERE id=$2`, now, authID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return p.GetAuthorization(ctx, authID)
}
