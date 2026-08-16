package store

import (
	"time"
	"visitor-parking/internal/model"
)

// restrictionActiveAt reports whether r is in effect at instant t, treating the
// window as half-open [EffectiveFrom, EffectiveTo).
func restrictionActiveAt(r *model.VehicleRestriction, t time.Time) bool {
	return !r.EffectiveFrom.After(t) && r.EffectiveTo.After(t)
}

// restrictionOverlaps reports whether r's window overlaps the half-open range
// [from, to).
func restrictionOverlaps(r *model.VehicleRestriction, from, to time.Time) bool {
	return r.EffectiveFrom.Before(to) && from.Before(r.EffectiveTo)
}

// restrictionInEffect reports whether r applies during [from, to). A degenerate
// range (from >= to) is treated as a point check at `from`, so callers can pass
// from==to==now for a single-instant lookup.
func restrictionInEffect(r *model.VehicleRestriction, from, to time.Time) bool {
	if !from.Before(to) {
		return restrictionActiveAt(r, from)
	}
	return restrictionOverlaps(r, from, to)
}

// pickRestriction returns the most severe active restriction that is in effect
// during [from, to) for the given plate: a forbidden restriction wins over a
// manual_confirm one. Returns nil if none match.
func pickRestriction(items []*model.VehicleRestriction, plate string, from, to time.Time) *model.VehicleRestriction {
	var forbidden, manual *model.VehicleRestriction
	for _, r := range items {
		if r == nil || r.ArchivedAt != nil || r.Status != model.RestrictionStatusActive {
			continue
		}
		if r.Plate != plate {
			continue
		}
		if !restrictionInEffect(r, from, to) {
			continue
		}
		switch r.Type {
		case model.RestrictionTypeForbidden:
			forbidden = r // forbidden always preferred over manual_confirm
		case model.RestrictionTypeManualConfirm:
			if manual == nil {
				manual = r
			}
		}
	}
	if forbidden != nil {
		return forbidden
	}
	return manual
}

func cloneRestriction(r *model.VehicleRestriction) *model.VehicleRestriction {
	c := *r
	c.ArchivedAt = clonePtr(r.ArchivedAt)
	return &c
}

const restrictionCols = "id,plate,type,effective_from,effective_to,reason,registered_by,status,archived_at,created_at,updated_at"

func scanRestriction(s scanner, r *model.VehicleRestriction) error {
	return s.Scan(&r.ID, &r.Plate, &r.Type, &r.EffectiveFrom, &r.EffectiveTo, &r.Reason, &r.RegisteredBy, &r.Status, &r.ArchivedAt, &r.CreatedAt, &r.UpdatedAt)
}
