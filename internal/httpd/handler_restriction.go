package httpd

import (
	"net/http"
	"visitor-parking/internal/service"
)

func (h *Handler) createRestriction(w http.ResponseWriter, r *http.Request) {
	var in service.CreateRestrictionInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	in.EffectiveFrom = parseTimeFlexible(in.EffectiveFrom)
	in.EffectiveTo = parseTimeFlexible(in.EffectiveTo)
	res, err := h.svc.CreateVehicleRestriction(r.Context(), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusCreated, res)
}

func (h *Handler) listRestrictions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := service.ListRestrictionFilter{
		Plate:        q.Get("plate"),
		Type:         q.Get("type"),
		Status:       q.Get("status"),
		RegisteredBy: q.Get("registered_by"),
		EffectiveOn:  q.Get("effective_on"),
	}
	f.Limit, f.Offset = parsePage(r)
	items, total, err := h.svc.ListVehicleRestrictions(r.Context(), f)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, paginate(items, total, f.Limit, f.Offset))
}

func (h *Handler) getRestriction(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.GetVehicleRestriction(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, res)
}

func (h *Handler) updateRestriction(w http.ResponseWriter, r *http.Request) {
	var in service.UpdateRestrictionInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	in.EffectiveFrom = parseTimeFlexible(in.EffectiveFrom)
	in.EffectiveTo = parseTimeFlexible(in.EffectiveTo)
	res, err := h.svc.UpdateVehicleRestriction(r.Context(), r.PathValue("id"), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, res)
}

func (h *Handler) archiveRestriction(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.ArchiveVehicleRestriction(r.Context(), r.PathValue("id")); err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusNoContent, nil)
}

func (h *Handler) releaseRestriction(w http.ResponseWriter, r *http.Request) {
	var in service.ReleaseInput
	_ = decodeJSON(r, &in)
	res, err := h.svc.ReleaseVehicleRestriction(r.Context(), r.PathValue("id"), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, res)
}

func (h *Handler) restrictionStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.svc.RestrictionStats(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, stats)
}
