package httpd
import (
	"net/http"
	"strings"
	"time"
	"visitor-parking/internal/model"
	"visitor-parking/internal/service"
)
type Handler struct {
	svc *service.Service
}
func NewHandler(svc *service.Service) *Handler {
	return &Handler{svc: svc}
}
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.healthz)
	mux.HandleFunc("POST /api/v1/residents", h.createResident)
	mux.HandleFunc("GET /api/v1/residents", h.listResidents)
	mux.HandleFunc("GET /api/v1/residents/{id}", h.getResident)
	mux.HandleFunc("PUT /api/v1/residents/{id}", h.updateResident)
	mux.HandleFunc("DELETE /api/v1/residents/{id}", h.archiveResident)
	mux.HandleFunc("POST /api/v1/vehicles", h.createVehicle)
	mux.HandleFunc("GET /api/v1/vehicles", h.listVehicles)
	mux.HandleFunc("GET /api/v1/vehicles/{id}", h.getVehicle)
	mux.HandleFunc("PUT /api/v1/vehicles/{id}", h.updateVehicle)
	mux.HandleFunc("DELETE /api/v1/vehicles/{id}", h.archiveVehicle)
	mux.HandleFunc("POST /api/v1/areas", h.createArea)
	mux.HandleFunc("GET /api/v1/areas", h.listAreas)
	mux.HandleFunc("GET /api/v1/areas/{id}", h.getArea)
	mux.HandleFunc("PUT /api/v1/areas/{id}", h.updateArea)
	mux.HandleFunc("DELETE /api/v1/areas/{id}", h.archiveArea)
	mux.HandleFunc("POST /api/v1/authorizations", h.createAuth)
	mux.HandleFunc("GET /api/v1/authorizations", h.listAuth)
	mux.HandleFunc("GET /api/v1/authorizations/{id}", h.getAuth)
	mux.HandleFunc("PUT /api/v1/authorizations/{id}", h.updateAuth)
	mux.HandleFunc("DELETE /api/v1/authorizations/{id}", h.archiveAuth)
	mux.HandleFunc("POST /api/v1/authorizations/{id}/revoke", h.revokeAuth)
	mux.HandleFunc("POST /api/v1/authorizations/{id}/entry", h.entry)
	mux.HandleFunc("POST /api/v1/authorizations/{id}/exit", h.exit)
	mux.HandleFunc("POST /api/v1/extension-applications", h.createExtensionApp)
	mux.HandleFunc("GET /api/v1/extension-applications", h.listExtensionApps)
	mux.HandleFunc("GET /api/v1/extension-applications/{id}", h.getExtensionApp)
	mux.HandleFunc("POST /api/v1/extension-applications/{id}/approve", h.approveExtensionApp)
	mux.HandleFunc("POST /api/v1/extension-applications/{id}/reject", h.rejectExtensionApp)
	mux.HandleFunc("POST /api/v1/extension-applications/{id}/revoke", h.revokeExtensionApp)
	mux.HandleFunc("GET /api/v1/records", h.listRecords)
	mux.HandleFunc("GET /api/v1/stats/current-vehicles", h.currentVehicles)
	mux.HandleFunc("GET /api/v1/stats/today-arrivals", h.todayArrivals)
	mux.HandleFunc("GET /api/v1/stats/expiring-soon", h.expiringSoon)
	mux.HandleFunc("GET /api/v1/stats/occupancy", h.occupancy)
	mux.HandleFunc("GET /api/v1/audit-logs", h.listAuditLogs)
	return mux
}
func (h *Handler) healthz(w http.ResponseWriter, r *http.Request) {
	writeOK(w, http.StatusOK, map[string]interface{}{"status": "ok", "time": time.Now().UTC()})
}
func (h *Handler) createResident(w http.ResponseWriter, r *http.Request) {
	var in service.CreateResidentInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	res, err := h.svc.CreateResident(r.Context(), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusCreated, res)
}
func (h *Handler) listResidents(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePage(r)
	items, total, err := h.svc.ListResidents(r.Context(), limit, offset)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, paginate(items, total, limit, offset))
}
func (h *Handler) getResident(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.GetResident(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, res)
}
func (h *Handler) updateResident(w http.ResponseWriter, r *http.Request) {
	var in service.UpdateResidentInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	res, err := h.svc.UpdateResident(r.Context(), r.PathValue("id"), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, res)
}
func (h *Handler) archiveResident(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.ArchiveResident(r.Context(), r.PathValue("id")); err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusNoContent, nil)
}
func (h *Handler) createVehicle(w http.ResponseWriter, r *http.Request) {
	var in service.CreateVehicleInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	v, err := h.svc.CreateVehicle(r.Context(), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusCreated, v)
}
func (h *Handler) listVehicles(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePage(r)
	items, total, err := h.svc.ListVehicles(r.Context(), limit, offset)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, paginate(items, total, limit, offset))
}
func (h *Handler) getVehicle(w http.ResponseWriter, r *http.Request) {
	v, err := h.svc.GetVehicle(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, v)
}
func (h *Handler) updateVehicle(w http.ResponseWriter, r *http.Request) {
	var v model.Vehicle
	if err := decodeJSON(r, &v); err != nil {
		writeErr(w, err)
		return
	}
	out, err := h.svc.UpdateVehicle(r.Context(), r.PathValue("id"), &v)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, out)
}
func (h *Handler) archiveVehicle(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.ArchiveVehicle(r.Context(), r.PathValue("id")); err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusNoContent, nil)
}
func (h *Handler) createArea(w http.ResponseWriter, r *http.Request) {
	var in service.CreateParkingAreaInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	a, err := h.svc.CreateParkingArea(r.Context(), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusCreated, a)
}
func (h *Handler) listAreas(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePage(r)
	items, total, err := h.svc.ListParkingAreas(r.Context(), limit, offset)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, paginate(items, total, limit, offset))
}
func (h *Handler) getArea(w http.ResponseWriter, r *http.Request) {
	a, err := h.svc.GetParkingArea(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, a)
}
func (h *Handler) updateArea(w http.ResponseWriter, r *http.Request) {
	var a model.ParkingArea
	if err := decodeJSON(r, &a); err != nil {
		writeErr(w, err)
		return
	}
	out, err := h.svc.UpdateParkingArea(r.Context(), r.PathValue("id"), &a)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, out)
}
func (h *Handler) archiveArea(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.ArchiveParkingArea(r.Context(), r.PathValue("id")); err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusNoContent, nil)
}
func (h *Handler) createAuth(w http.ResponseWriter, r *http.Request) {
	var in service.CreateAuthorizationInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	in.StartTime = parseTimeFlexible(in.StartTime)
	in.EndTime = parseTimeFlexible(in.EndTime)
	a, err := h.svc.CreateAuthorization(r.Context(), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusCreated, a)
}
func (h *Handler) listAuth(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := service.ListAuthFilter{
		Building:      q.Get("building"),
		Plate:         q.Get("plate"),
		ParkingAreaID: q.Get("area_id"),
		Status:        q.Get("status"),
		ValidOn:       q.Get("valid_on"),
		Today:         q.Get("today") == "1" || q.Get("today") == "true",
		ExpiringSoon:  q.Get("expiring_soon") == "1" || q.Get("expiring_soon") == "true",
	}
	f.Limit, f.Offset = parsePage(r)
	items, total, err := h.svc.ListAuthorizations(r.Context(), f)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, paginate(items, total, f.Limit, f.Offset))
}
func (h *Handler) getAuth(w http.ResponseWriter, r *http.Request) {
	a, err := h.svc.GetAuthorization(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, a)
}
func (h *Handler) updateAuth(w http.ResponseWriter, r *http.Request) {
	var in model.Authorization
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	in.StartTime = parseTimeFlexible(in.StartTime)
	in.EndTime = parseTimeFlexible(in.EndTime)
	out, err := h.svc.UpdateAuthorization(r.Context(), r.PathValue("id"), &in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, out)
}
func (h *Handler) archiveAuth(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.ArchiveAuthorization(r.Context(), r.PathValue("id")); err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusNoContent, nil)
}
func (h *Handler) revokeAuth(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Operator string `json:"operator"`
		Reason   string `json:"reason"`
	}
	_ = decodeJSON(r, &body)
	a, err := h.svc.Revoke(r.Context(), r.PathValue("id"),
		strings.TrimSpace(body.Operator), body.Reason)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, a)
}
func (h *Handler) entry(w http.ResponseWriter, r *http.Request) {
	rec, err := h.svc.EnterVehicle(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusCreated, rec)
}
func (h *Handler) exit(w http.ResponseWriter, r *http.Request) {
	var in service.ExitRequest
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	rec, err := h.svc.ExitVehicle(r.Context(), r.PathValue("id"), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, rec)
}
func (h *Handler) createExtensionApp(w http.ResponseWriter, r *http.Request) {
	var in service.CreateExtensionInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	in.NewEndTime = parseTimeFlexible(in.NewEndTime)
	app, err := h.svc.CreateExtensionApplication(r.Context(), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusCreated, app)
}
func (h *Handler) listExtensionApps(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := service.ListExtensionFilter{
		AuthorizationID: q.Get("authorization_id"),
		Plate:           q.Get("plate"),
		Status:          q.Get("status"),
		Applicant:       q.Get("applicant"),
	}
	f.Limit, f.Offset = parsePage(r)
	items, total, err := h.svc.ListExtensionApplications(r.Context(), f)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, paginate(items, total, f.Limit, f.Offset))
}
func (h *Handler) getExtensionApp(w http.ResponseWriter, r *http.Request) {
	app, err := h.svc.GetExtensionApplication(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, app)
}
func (h *Handler) approveExtensionApp(w http.ResponseWriter, r *http.Request) {
	var in service.ApproveExtensionInput
	_ = decodeJSON(r, &in)
	app, auth, err := h.svc.ApproveExtensionApplication(r.Context(), r.PathValue("id"), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, map[string]interface{}{"application": app, "authorization": auth})
}
func (h *Handler) rejectExtensionApp(w http.ResponseWriter, r *http.Request) {
	var in service.RejectExtensionInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, err)
		return
	}
	app, err := h.svc.RejectExtensionApplication(r.Context(), r.PathValue("id"), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, app)
}
func (h *Handler) revokeExtensionApp(w http.ResponseWriter, r *http.Request) {
	var in service.RevokeExtensionInput
	_ = decodeJSON(r, &in)
	app, err := h.svc.RevokeExtensionApplication(r.Context(), r.PathValue("id"), in)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, app)
}
func (h *Handler) listRecords(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, offset := parsePage(r)
	items, total, err := h.svc.ListRecords(r.Context(), q.Get("area_id"), q.Get("status"), q.Get("plate"), limit, offset)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, paginate(items, total, limit, offset))
}
func (h *Handler) currentVehicles(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePage(r)
	items, total, err := h.svc.ListCurrentVehicles(r.Context(), r.URL.Query().Get("area_id"), limit, offset)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, paginate(items, total, limit, offset))
}
func (h *Handler) todayArrivals(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePage(r)
	items, total, err := h.svc.TodayArrivals(r.Context(), limit, offset)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, paginate(items, total, limit, offset))
}
func (h *Handler) expiringSoon(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePage(r)
	items, total, err := h.svc.ExpiringSoon(r.Context(), limit, offset)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, paginate(items, total, limit, offset))
}
func (h *Handler) occupancy(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.AreaOccupancy(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, map[string]interface{}{"areas": items})
}
func (h *Handler) listAuditLogs(w http.ResponseWriter, r *http.Request) {
	limit, offset := parsePage(r)
	items, err := h.svc.ListAuditLogs(r.Context(), r.URL.Query().Get("entity_type"), limit, offset)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeOK(w, http.StatusOK, paginate(items, int64(len(items)), limit, offset))
}
func parseTimeFlexible(t time.Time) time.Time {
	return t
}
func pageNo(limit, offset int) int {
	if limit <= 0 {
		limit = 20
	}
	return offset/limit + 1
}
type pageResult struct {
	Items interface{} `json:"items"`
	Total int64       `json:"total"`
	Page  int         `json:"page"`
	Size  int         `json:"size"`
}
func paginate(items interface{}, total int64, limit, offset int) pageResult {
	return pageResult{Items: items, Total: total, Page: pageNo(limit, offset), Size: limit}
}
