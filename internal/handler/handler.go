package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pasya/time-recording/internal/domain"
	"github.com/pasya/time-recording/internal/service"
)

// TimeHandler wires HTTP routes to the TimeService
type TimeHandler struct {
	svc *service.TimeService
}

func NewTimeHandler(svc *service.TimeService) *TimeHandler {
	return &TimeHandler{svc: svc}
}

// RegisterRoutes attaches all handlers to a mux
func (h *TimeHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", h.handleHealth)

	// Clock events
	mux.HandleFunc("/clock-in", h.handleClockIn)
	mux.HandleFunc("/clock-out", h.handleClockOut)

	// CRUD on records
	mux.HandleFunc("/records", h.handleRecords)
	mux.HandleFunc("/records/", h.handleRecordByID)

	// Report
	mux.HandleFunc("/report", h.handleReport)
}

// ---- helpers ----

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decodeBody(r *http.Request, dst any) error {
	return json.NewDecoder(r.Body).Decode(dst)
}

func mapDomainErr(err error) (int, string) {
	switch {
	case errors.Is(err, domain.ErrAlreadyClockedIn):
		return http.StatusConflict, err.Error()
	case errors.Is(err, domain.ErrNotClockedIn):
		return http.StatusConflict, err.Error()
	case errors.Is(err, domain.ErrRecordNotFound):
		return http.StatusNotFound, err.Error()
	case errors.Is(err, domain.ErrInvalidTimeRange):
		return http.StatusBadRequest, err.Error()
	case errors.Is(err, domain.ErrOverlappingRecord):
		return http.StatusConflict, err.Error()
	default:
		return http.StatusInternalServerError, "internal server error"
	}
}

// ---- handlers ----

func (h *TimeHandler) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// POST /clock-in
type clockInRequest struct {
	UserID string `json:"user_id"`
	At     string `json:"at,omitempty"` // ISO8601, defaults to now
	Note   string `json:"note,omitempty"`
}

func (h *TimeHandler) handleClockIn(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req clockInRequest
	if err := decodeBody(r, &req); err != nil || req.UserID == "" {
		writeError(w, http.StatusBadRequest, "user_id is required")
		return
	}
	at, err := parseTime(req.At)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid 'at' timestamp")
		return
	}
	rec, err := h.svc.ClockIn(req.UserID, req.Note, at)
	if err != nil {
		code, msg := mapDomainErr(err)
		writeError(w, code, msg)
		return
	}
	writeJSON(w, http.StatusCreated, rec)
}

// POST /clock-out
type clockOutRequest struct {
	UserID string `json:"user_id"`
	At     string `json:"at,omitempty"`
	Note   string `json:"note,omitempty"`
}

func (h *TimeHandler) handleClockOut(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req clockOutRequest
	if err := decodeBody(r, &req); err != nil || req.UserID == "" {
		writeError(w, http.StatusBadRequest, "user_id is required")
		return
	}
	at, err := parseTime(req.At)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid 'at' timestamp")
		return
	}
	rec, err := h.svc.ClockOut(req.UserID, req.Note, at)
	if err != nil {
		code, msg := mapDomainErr(err)
		writeError(w, code, msg)
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

// POST /records  – manual create
// GET  /records?user_id=&from=&to=
type createRecordRequest struct {
	UserID   string `json:"user_id"`
	ClockIn  string `json:"clock_in"`
	ClockOut string `json:"clock_out,omitempty"`
	Note     string `json:"note,omitempty"`
}

func (h *TimeHandler) handleRecords(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req createRecordRequest
		if err := decodeBody(r, &req); err != nil || req.UserID == "" || req.ClockIn == "" {
			writeError(w, http.StatusBadRequest, "user_id and clock_in are required")
			return
		}
		ci, err := time.Parse(time.RFC3339, req.ClockIn)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid clock_in format")
			return
		}
		var co time.Time
		if req.ClockOut != "" {
			co, err = time.Parse(time.RFC3339, req.ClockOut)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid clock_out format")
				return
			}
		}
		rec, err := h.svc.CreateRecord(req.UserID, ci, co, req.Note)
		if err != nil {
			code, msg := mapDomainErr(err)
			writeError(w, code, msg)
			return
		}
		writeJSON(w, http.StatusCreated, rec)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// GET    /records/{id}
// PUT    /records/{id}
// DELETE /records/{id}
type updateRecordRequest struct {
	ClockIn  string `json:"clock_in"`
	ClockOut string `json:"clock_out,omitempty"`
	Note     string `json:"note,omitempty"`
}

func (h *TimeHandler) handleRecordByID(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/records/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid record id")
		return
	}

	switch r.Method {
	case http.MethodGet:
		rec, err := h.svc.GetRecord(id)
		if err != nil {
			code, msg := mapDomainErr(err)
			writeError(w, code, msg)
			return
		}
		writeJSON(w, http.StatusOK, rec)

	case http.MethodPut:
		var req updateRecordRequest
		if err := decodeBody(r, &req); err != nil || req.ClockIn == "" {
			writeError(w, http.StatusBadRequest, "clock_in is required")
			return
		}
		ci, err := time.Parse(time.RFC3339, req.ClockIn)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid clock_in format")
			return
		}
		var co time.Time
		if req.ClockOut != "" {
			co, err = time.Parse(time.RFC3339, req.ClockOut)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid clock_out format")
				return
			}
		}
		rec, err := h.svc.UpdateRecord(id, ci, co, req.Note)
		if err != nil {
			code, msg := mapDomainErr(err)
			writeError(w, code, msg)
			return
		}
		writeJSON(w, http.StatusOK, rec)

	case http.MethodDelete:
		if err := h.svc.DeleteRecord(id); err != nil {
			code, msg := mapDomainErr(err)
			writeError(w, code, msg)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// GET /report?user_id=&from=2024-01-01&to=2024-01-31
func (h *TimeHandler) handleReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	q := r.URL.Query()
	userID := q.Get("user_id")
	fromStr := q.Get("from")
	toStr := q.Get("to")

	if userID == "" || fromStr == "" || toStr == "" {
		writeError(w, http.StatusBadRequest, "user_id, from, and to are required")
		return
	}

	from, err := time.Parse("2006-01-02", fromStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "from must be YYYY-MM-DD")
		return
	}
	to, err := time.Parse("2006-01-02", toStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "to must be YYYY-MM-DD")
		return
	}
	if to.Before(from) {
		writeError(w, http.StatusBadRequest, "to must not be before from")
		return
	}

	report, err := h.svc.GenerateReport(userID, from, to)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate report")
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// parseTime parses an optional RFC3339 string; defaults to time.Now()
func parseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Now(), nil
	}
	return time.Parse(time.RFC3339, s)
}
