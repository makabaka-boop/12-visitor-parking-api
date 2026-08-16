package httpd
import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"visitor-parking/internal/service"
	"visitor-parking/internal/store"
)
type envelope struct {
	Code    int         `json:"code"` // HTTP status
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *errBody    `json:"error,omitempty"`
}
type errBody struct {
	Message string               `json:"message"`
	Fields  []service.FieldError `json:"fields,omitempty"`
}
func writeOK(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{Code: status, Success: true, Data: data})
}
func writeErr(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	msg := err.Error()
	body := &errBody{Message: msg}
	var fe *service.FieldError
	if errors.As(err, &fe) {
		status = http.StatusBadRequest
		body.Message = "validation failed"
		body.Fields = []service.FieldError{*fe}
	}
	switch {
	case errors.Is(err, store.ErrNotFound):
		status = http.StatusNotFound
		body.Message = "resource not found"
	case errors.Is(err, store.ErrConflict):
		status = http.StatusConflict
		body.Message = "conflicting request"
	case errors.Is(err, store.ErrAlreadyEntered):
		status = http.StatusConflict
		body.Message = "vehicle already entered"
	case errors.Is(err, store.ErrAlreadyExited):
		status = http.StatusConflict
		body.Message = "vehicle already exited"
	case errors.Is(err, store.ErrConcurrentModify):
		status = http.StatusConflict
		body.Message = "resource was modified by another request, please refresh and retry"
	case errors.Is(err, store.ErrStatusTransition):
		status = http.StatusConflict
		body.Message = "invalid status transition for current state"
	case errors.Is(err, store.ErrNoCapacity):
		status = http.StatusConflict
		body.Message = "parking area is full"
	case errors.Is(err, store.ErrOutOfTimeWindow):
		status = http.StatusConflict
		body.Message = "authorization is outside its valid time window"
	case errors.Is(err, store.ErrResidentDisabled):
		status = http.StatusBadRequest
		body.Message = "resident is disabled"
	case errors.Is(err, store.ErrAreaArchived):
		status = http.StatusBadRequest
		body.Message = "parking area is archived"
	case errors.Is(err, store.ErrPlateForbidden):
		status = http.StatusForbidden
		body.Message = "vehicle is forbidden from entering"
	case errors.Is(err, store.ErrManualConfirmRequired):
		status = http.StatusBadRequest
		body.Message = "vehicle requires manual confirmation; provide confirmer"
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope{Code: status, Success: false, Error: body})
}
func decodeJSON(r *http.Request, v interface{}) error {
	if r.Body == nil {
		return nil
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return nil // empty body is valid for optional-body endpoints (e.g. entry)
		}
		return service.NewFieldError("body", err.Error())
	}
	return nil
}
func parsePage(r *http.Request) (int, int) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	return limit, offset
}
