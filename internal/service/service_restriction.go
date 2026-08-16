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

// EntryRequest is the optional body for the vehicle-entry endpoint.
type EntryRequest struct {
	Confirmer string `json:"confirmer"` // required only when the plate is under a manual_confirm restriction
}

// auditRestrictionCheck records the outcome of a restriction-list check that ran
// during authorization creation or vehicle entry. It is best-effort: audit-log
// write failures are ignored. Only invoked when a restriction was matched.
func (s *Service) auditRestrictionCheck(ctx context.Context, scope, plate, operator string, r *model.VehicleRestriction, confirmer string, err error) {
	if r == nil {
		return
	}
	var detail string
	switch {
	case errors.Is(err, store.ErrPlateForbidden):
		detail = fmt.Sprintf("%s: plate %s forbidden by restriction %s, denied", scope, plate, r.ID)
	case errors.Is(err, store.ErrManualConfirmRequired):
		detail = fmt.Sprintf("%s: plate %s requires manual confirmation (restriction %s), confirmer missing", scope, plate, r.ID)
	case r.Type == model.RestrictionTypeManualConfirm:
		detail = fmt.Sprintf("%s: plate %s manual-confirmed by %s against restriction %s", scope, plate, confirmer, r.ID)
	default:
		detail = fmt.Sprintf("%s: plate %s restriction %s checked", scope, plate, r.ID)
	}
	s.audit(ctx, "restriction.check", "restriction", r.ID, operator, detail)
}

// CreateRestrictionInput is the request body for creating a vehicle restriction.
type CreateRestrictionInput struct {
	Plate         string    `json:"plate"`
	Type          string    `json:"type"`
	EffectiveFrom time.Time `json:"effective_from"`
	EffectiveTo   time.Time `json:"effective_to"`
	Reason        string    `json:"reason"`
	RegisteredBy  string    `json:"registered_by"`
}

func (s *Service) CreateVehicleRestriction(ctx context.Context, in CreateRestrictionInput) (*model.VehicleRestriction, error) {
	p := plate.Normalize(in.Plate)
	if !plate.Valid(p) {
		return nil, newFieldError("plate", "invalid plate format")
	}
	if in.Type != model.RestrictionTypeForbidden && in.Type != model.RestrictionTypeManualConfirm {
		return nil, newFieldError("type", "must be 'forbidden' or 'manual_confirm'")
	}
	if in.EffectiveFrom.IsZero() || in.EffectiveTo.IsZero() {
		return nil, newFieldError("effective_from", "effective_from and effective_to are required")
	}
	if !in.EffectiveTo.After(in.EffectiveFrom) {
		return nil, newFieldError("effective_to", "must be later than effective_from")
	}
	now := s.nowT()
	r := &model.VehicleRestriction{
		ID:            store.NewID("rstr"),
		Plate:         p,
		Type:          in.Type,
		EffectiveFrom: in.EffectiveFrom,
		EffectiveTo:   in.EffectiveTo,
		Reason:        strings.TrimSpace(in.Reason),
		RegisteredBy:  defStr(strings.TrimSpace(in.RegisteredBy), "system"),
		Status:        model.RestrictionStatusActive,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.store.CreateVehicleRestriction(ctx, r, now); err != nil {
		return nil, err
	}
	s.audit(ctx, "restriction.create", "restriction", r.ID, r.RegisteredBy, fmt.Sprintf("restriction %s created for plate %s", r.Type, r.Plate))
	return r, nil
}

func (s *Service) GetVehicleRestriction(ctx context.Context, id string) (*model.VehicleRestriction, error) {
	return s.store.GetVehicleRestriction(ctx, id)
}

// UpdateRestrictionInput is the request body for updating a vehicle restriction.
// Only active restrictions may be updated. Requires updated_at for optimistic
// concurrency control.
type UpdateRestrictionInput struct {
	Type          string    `json:"type"`
	EffectiveFrom time.Time `json:"effective_from"`
	EffectiveTo   time.Time `json:"effective_to"`
	Reason        string    `json:"reason"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (s *Service) UpdateVehicleRestriction(ctx context.Context, id string, in UpdateRestrictionInput) (*model.VehicleRestriction, error) {
	if in.UpdatedAt.IsZero() {
		return nil, newFieldError("updated_at", "is required for concurrency check")
	}
	if in.Type != "" && in.Type != model.RestrictionTypeForbidden && in.Type != model.RestrictionTypeManualConfirm {
		return nil, newFieldError("type", "must be 'forbidden' or 'manual_confirm'")
	}
	cur, err := s.store.GetVehicleRestriction(ctx, id)
	if err != nil {
		return nil, err
	}
	if !in.UpdatedAt.Equal(cur.UpdatedAt) {
		return nil, store.ErrConcurrentModify
	}
	if in.EffectiveFrom.IsZero() {
		in.EffectiveFrom = cur.EffectiveFrom
	}
	if in.EffectiveTo.IsZero() {
		in.EffectiveTo = cur.EffectiveTo
	}
	if !in.EffectiveTo.After(in.EffectiveFrom) {
		return nil, newFieldError("effective_to", "must be later than effective_from")
	}
	cur.EffectiveFrom = in.EffectiveFrom
	cur.EffectiveTo = in.EffectiveTo
	if in.Type != "" {
		cur.Type = in.Type
	}
	if in.Reason != "" {
		cur.Reason = in.Reason
	}
	cur.UpdatedAt = s.nowT()
	if err := s.store.UpdateVehicleRestriction(ctx, cur); err != nil {
		return nil, err
	}
	s.audit(ctx, "restriction.update", "restriction", id, "system", "restriction updated")
	return cur, nil
}

// ReleaseInput is the request body for releasing (lifting) a restriction.
type ReleaseInput struct {
	Operator string `json:"operator"`
	Reason   string `json:"reason"`
}

// ReleaseVehicleRestriction lifts an active restriction. The historical record
// is retained (status becomes 'released'); it is never deleted.
func (s *Service) ReleaseVehicleRestriction(ctx context.Context, id string, in ReleaseInput) (*model.VehicleRestriction, error) {
	operator := strings.TrimSpace(in.Operator)
	now := s.nowT()
	r, err := s.store.ReleaseVehicleRestriction(ctx, id, now, operator, in.Reason)
	if err != nil {
		return nil, err
	}
	s.audit(ctx, "restriction.release", "restriction", id, defStr(operator, "system"), "restriction released: "+in.Reason)
	return r, nil
}

func (s *Service) ArchiveVehicleRestriction(ctx context.Context, id string) error {
	if err := s.store.ArchiveVehicleRestriction(ctx, id, s.nowT()); err != nil {
		return err
	}
	s.audit(ctx, "restriction.archive", "restriction", id, "system", "restriction archived")
	return nil
}

// ListRestrictionFilter mirrors the query-string filter for the restriction list.
type ListRestrictionFilter struct {
	Plate         string
	Type          string
	Status        string
	RegisteredBy  string
	EffectiveOn   string // RFC3339
	Limit, Offset int
}

func (s *Service) ListVehicleRestrictions(ctx context.Context, f ListRestrictionFilter) ([]*model.VehicleRestriction, int64, error) {
	storeFilter := model.RestrictionFilter{
		Plate:        plate.Normalize(f.Plate),
		Type:         f.Type,
		Status:       f.Status,
		RegisteredBy: f.RegisteredBy,
		Page:         normPage(f.Limit, f.Offset),
	}
	if f.EffectiveOn != "" {
		if t, err := time.Parse(time.RFC3339, f.EffectiveOn); err == nil {
			storeFilter.EffectiveOn = &t
		}
	}
	return s.store.ListVehicleRestrictions(ctx, storeFilter)
}

func (s *Service) RestrictionStats(ctx context.Context) (*model.RestrictionStats, error) {
	return s.store.RestrictionStats(ctx, s.nowT())
}
