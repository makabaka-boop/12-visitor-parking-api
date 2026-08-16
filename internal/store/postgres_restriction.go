package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
	"visitor-parking/internal/model"
)

// lockedRestriction loads the most severe active restriction (forbidden
// preferred over manual_confirm) for plate that is in effect during [from, to).
// A degenerate range (from >= to) is treated as a point check at `from`. It must
// run inside the caller's transaction so the read is serialized with the
// subsequent write (no TOCTOU between the check and the create/entry). Returns
// nil (no error) when no active restriction matches.
func (p *Postgres) lockedRestriction(ctx context.Context, plate string, from, to time.Time) (*model.VehicleRestriction, error) {
	var rows *sql.Rows
	var err error
	if !from.Before(to) {
		// point check at `from`: half-open [effective_from, effective_to)
		rows, err = p.q(ctx).QueryContext(ctx,
			`SELECT `+restrictionCols+` FROM vehicle_restrictions
			 WHERE plate=$1 AND archived_at IS NULL AND status='active'
			   AND effective_from <= $2 AND $2 < effective_to
			 ORDER BY (type='forbidden') DESC`, plate, from)
	} else {
		rows, err = p.q(ctx).QueryContext(ctx,
			`SELECT `+restrictionCols+` FROM vehicle_restrictions
			 WHERE plate=$1 AND archived_at IS NULL AND status='active'
			   AND effective_from < $2 AND $3 < effective_to
			 ORDER BY (type='forbidden') DESC`, plate, to, from)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var forbidden, manual *model.VehicleRestriction
	for rows.Next() {
		r := &model.VehicleRestriction{}
		if err := scanRestriction(rows, r); err != nil {
			return nil, err
		}
		switch r.Type {
		case model.RestrictionTypeForbidden:
			if forbidden == nil {
				forbidden = r
			}
		case model.RestrictionTypeManualConfirm:
			if manual == nil {
				manual = r
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if forbidden != nil {
		return forbidden, nil
	}
	return manual, nil
}

func (p *Postgres) CreateVehicleRestriction(ctx context.Context, r *model.VehicleRestriction, now time.Time) error {
	return p.runTx(ctx, func(ctx context.Context) error {
		var n int64
		if err := p.q(ctx).QueryRowContext(ctx,
			`SELECT count(*) FROM vehicle_restrictions
			 WHERE plate=$1 AND archived_at IS NULL AND status='active'
			   AND effective_from < $2 AND $3 < effective_to`,
			r.Plate, r.EffectiveTo, r.EffectiveFrom).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return ErrConflict
		}
		_, err := p.q(ctx).ExecContext(ctx, `INSERT INTO vehicle_restrictions (`+restrictionCols+`) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
			r.ID, r.Plate, r.Type, r.EffectiveFrom, r.EffectiveTo, r.Reason, r.RegisteredBy, r.Status, r.ArchivedAt, r.CreatedAt, r.UpdatedAt)
		if err != nil && isUniqueViolation(err) {
			return ErrConflict
		}
		return err
	})
}

func (p *Postgres) GetVehicleRestriction(ctx context.Context, id string) (*model.VehicleRestriction, error) {
	r := &model.VehicleRestriction{}
	err := scanRestriction(p.q(ctx).QueryRowContext(ctx, `SELECT `+restrictionCols+` FROM vehicle_restrictions WHERE id=$1 AND archived_at IS NULL`, id), r)
	return r, mapNotFound(err)
}

func (p *Postgres) UpdateVehicleRestriction(ctx context.Context, r *model.VehicleRestriction) error {
	return p.runTx(ctx, func(ctx context.Context) error {
		var status string
		if err := p.q(ctx).QueryRowContext(ctx, `SELECT status FROM vehicle_restrictions WHERE id=$1 AND archived_at IS NULL FOR UPDATE`, r.ID).Scan(&status); err != nil {
			return mapNotFound(err)
		}
		if status != model.RestrictionStatusActive {
			return ErrStatusTransition
		}
		var n int64
		if err := p.q(ctx).QueryRowContext(ctx,
			`SELECT count(*) FROM vehicle_restrictions
			 WHERE plate=$1 AND archived_at IS NULL AND status='active' AND id <> $2
			   AND effective_from < $3 AND $4 < effective_to`,
			r.Plate, r.ID, r.EffectiveTo, r.EffectiveFrom).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return ErrConflict
		}
		res, err := p.q(ctx).ExecContext(ctx,
			`UPDATE vehicle_restrictions SET type=$1,effective_from=$2,effective_to=$3,reason=$4,updated_at=$5
			 WHERE id=$6 AND archived_at IS NULL AND updated_at=$7`,
			r.Type, r.EffectiveFrom, r.EffectiveTo, r.Reason, r.UpdatedAt, r.ID, r.UpdatedAt)
		return wrap(res, err, ErrConcurrentModify)
	})
}

func (p *Postgres) ReleaseVehicleRestriction(ctx context.Context, id string, now time.Time, operator, reason string) (*model.VehicleRestriction, error) {
	err := p.runTx(ctx, func(ctx context.Context) error {
		var status string
		if err := p.q(ctx).QueryRowContext(ctx, `SELECT status FROM vehicle_restrictions WHERE id=$1 AND archived_at IS NULL FOR UPDATE`, id).Scan(&status); err != nil {
			return mapNotFound(err)
		}
		if status != model.RestrictionStatusActive {
			return ErrStatusTransition
		}
		_, err := p.q(ctx).ExecContext(ctx, `UPDATE vehicle_restrictions SET status='released',updated_at=$1 WHERE id=$2`, now, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	return p.GetVehicleRestriction(ctx, id)
}

func (p *Postgres) ArchiveVehicleRestriction(ctx context.Context, id string, now time.Time) error {
	res, err := p.q(ctx).ExecContext(ctx, `UPDATE vehicle_restrictions SET archived_at=$1,updated_at=$2 WHERE id=$3 AND archived_at IS NULL`, now, now, id)
	return wrap(res, err, ErrNotFound)
}

func (p *Postgres) ListVehicleRestrictions(ctx context.Context, f model.RestrictionFilter) ([]*model.VehicleRestriction, int64, error) {
	var sb strings.Builder
	args := []interface{}{}
	sb.WriteString(`SELECT count(*) OVER(),` + restrictionCols + ` FROM vehicle_restrictions WHERE archived_at IS NULL`)
	n := 1
	add := func(cond string, arg interface{}) {
		sb.WriteString(" AND " + fmt.Sprintf(cond, n))
		args = append(args, arg)
		n++
	}
	if f.Plate != "" {
		add("plate=$%d", f.Plate)
	}
	if f.Type != "" {
		add("type=$%d", f.Type)
	}
	if f.Status != "" {
		add("status=$%d", f.Status)
	}
	if f.RegisteredBy != "" {
		add("registered_by=$%d", f.RegisteredBy)
	}
	if f.EffectiveOn != nil {
		// half-open point check: effective_from <= t AND t < effective_to,
		// both placeholders bound to the same argument.
		sb.WriteString(fmt.Sprintf(" AND effective_from <= $%d AND $%d < effective_to", n, n))
		args = append(args, *f.EffectiveOn)
		n++
	}
	sb.WriteString(" ORDER BY effective_from DESC")
	page := normPage(f.Page)
	sb.WriteString(fmt.Sprintf(" LIMIT $%d OFFSET $%d", n, n+1))
	args = append(args, page.Limit, page.Offset)
	rows, err := p.db.QueryContext(ctx, sb.String(), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*model.VehicleRestriction
	var total int64
	for rows.Next() {
		r := &model.VehicleRestriction{}
		if err := scanRestrictionPageRow(rows, &total, r); err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	return out, total, rows.Err()
}

func (p *Postgres) RestrictionStats(ctx context.Context, now time.Time) (*model.RestrictionStats, error) {
	st := &model.RestrictionStats{}
	q := `SELECT
		(SELECT count(*) FROM vehicle_restrictions WHERE archived_at IS NULL AND status='active'),
		(SELECT count(*) FROM vehicle_restrictions WHERE archived_at IS NULL AND status='active' AND effective_from <= $1 AND $1 < effective_to),
		(SELECT count(*) FROM vehicle_restrictions WHERE archived_at IS NULL AND status='active' AND type='forbidden'),
		(SELECT count(*) FROM vehicle_restrictions WHERE archived_at IS NULL AND status='active' AND type='manual_confirm'),
		(SELECT count(*) FROM vehicle_restrictions WHERE archived_at IS NULL AND status='released')`
	if err := p.db.QueryRowContext(ctx, q, now).Scan(&st.TotalActive, &st.CurrentlyInEffect, &st.Forbidden, &st.ManualConfirm, &st.Released); err != nil {
		return nil, err
	}
	return st, nil
}
