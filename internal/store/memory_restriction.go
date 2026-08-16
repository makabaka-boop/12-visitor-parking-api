package store

import (
	"context"
	"sort"
	"time"
	"visitor-parking/internal/model"
)

// findActiveRestriction returns the most severe active restriction (forbidden
// preferred over manual_confirm) for plate that is in effect during [from, to).
// A degenerate range (from >= to) is treated as a point check at `from`. The
// caller must hold m.mu. The returned pointer aliases an internal value; callers
// clone before returning it across the store boundary.
func (m *Memory) findActiveRestriction(plate string, from, to time.Time) *model.VehicleRestriction {
	items := make([]*model.VehicleRestriction, 0, len(m.restrictions))
	for _, r := range m.restrictions {
		items = append(items, r)
	}
	return pickRestriction(items, plate, from, to)
}

// overlapsActiveRestriction reports whether the proposed active restriction
// window overlaps any existing active restriction for the same plate. Used by
// CreateVehicleRestriction and UpdateVehicleRestriction to enforce "no
// duplicate active records for the same plate in the same time range". When
// excludeID is non-empty that record is skipped (so an update of itself does
// not trip the guard). The caller must hold m.mu.
func (m *Memory) overlapsActiveRestriction(plate string, from, to time.Time, excludeID string) bool {
	for _, r := range m.restrictions {
		if r.ArchivedAt != nil || r.Status != model.RestrictionStatusActive {
			continue
		}
		if r.Plate != plate || r.ID == excludeID {
			continue
		}
		if r.EffectiveFrom.Before(to) && from.Before(r.EffectiveTo) {
			return true
		}
	}
	return false
}

func (m *Memory) CreateVehicleRestriction(ctx context.Context, r *model.VehicleRestriction, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.restrictions[r.ID]; ok {
		return ErrConflict
	}
	if m.overlapsActiveRestriction(r.Plate, r.EffectiveFrom, r.EffectiveTo, "") {
		return ErrConflict
	}
	m.restrictions[r.ID] = cloneRestriction(r)
	return nil
}

func (m *Memory) GetVehicleRestriction(ctx context.Context, id string) (*model.VehicleRestriction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.restrictions[id]
	if !ok || r.ArchivedAt != nil {
		return nil, ErrNotFound
	}
	return cloneRestriction(r), nil
}

func (m *Memory) UpdateVehicleRestriction(ctx context.Context, r *model.VehicleRestriction) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, ok := m.restrictions[r.ID]
	if !ok || cur.ArchivedAt != nil {
		return ErrNotFound
	}
	if !cur.UpdatedAt.Equal(r.UpdatedAt) {
		return ErrConcurrentModify
	}
	if cur.Status != model.RestrictionStatusActive {
		return ErrStatusTransition
	}
	if m.overlapsActiveRestriction(r.Plate, r.EffectiveFrom, r.EffectiveTo, r.ID) {
		return ErrConflict
	}
	m.restrictions[r.ID] = cloneRestriction(r)
	return nil
}

func (m *Memory) ReleaseVehicleRestriction(ctx context.Context, id string, now time.Time, operator, reason string) (*model.VehicleRestriction, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.restrictions[id]
	if !ok || r.ArchivedAt != nil {
		return nil, ErrNotFound
	}
	if r.Status != model.RestrictionStatusActive {
		return nil, ErrStatusTransition
	}
	r.Status = model.RestrictionStatusReleased
	r.UpdatedAt = now
	m.restrictions[id] = cloneRestriction(r)
	// operator/reason are recorded by the service via the audit log; the
	// restriction row itself retains its original reason (history preserved).
	return cloneRestriction(r), nil
}

func (m *Memory) ArchiveVehicleRestriction(ctx context.Context, id string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.restrictions[id]
	if !ok || r.ArchivedAt != nil {
		return ErrNotFound
	}
	c := now
	r.ArchivedAt = &c
	r.UpdatedAt = now
	m.restrictions[id] = cloneRestriction(r)
	return nil
}

func (m *Memory) ListVehicleRestrictions(ctx context.Context, f model.RestrictionFilter) ([]*model.VehicleRestriction, int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*model.VehicleRestriction, 0, len(m.restrictions))
	for _, r := range m.restrictions {
		if r.ArchivedAt != nil {
			continue
		}
		if f.Plate != "" && r.Plate != f.Plate {
			continue
		}
		if f.Type != "" && r.Type != f.Type {
			continue
		}
		if f.Status != "" && r.Status != f.Status {
			continue
		}
		if f.RegisteredBy != "" && r.RegisteredBy != f.RegisteredBy {
			continue
		}
		if f.EffectiveOn != nil && !restrictionActiveAt(r, *f.EffectiveOn) {
			continue
		}
		out = append(out, cloneRestriction(r))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EffectiveFrom.After(out[j].EffectiveFrom) })
	return pageOf(out, f.Page), int64(len(out)), nil
}

func (m *Memory) RestrictionStats(ctx context.Context, now time.Time) (*model.RestrictionStats, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := &model.RestrictionStats{}
	for _, r := range m.restrictions {
		if r.ArchivedAt != nil {
			continue
		}
		switch r.Status {
		case model.RestrictionStatusActive:
			st.TotalActive++
			if restrictionActiveAt(r, now) {
				st.CurrentlyInEffect++
			}
			switch r.Type {
			case model.RestrictionTypeForbidden:
				st.Forbidden++
			case model.RestrictionTypeManualConfirm:
				st.ManualConfirm++
			}
		case model.RestrictionStatusReleased:
			st.Released++
		}
	}
	return st, nil
}
