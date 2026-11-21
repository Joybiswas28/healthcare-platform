package appointment

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"healthcare-platform/internal/auth"
	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/platform/httpx"
)

type Handler struct {
	store *Store
	log   *slog.Logger
}

func NewHandler(store *Store, log *slog.Logger) *Handler {
	return &Handler{store: store, log: log}
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	httpx.WriteError(w, r, h.log, err)
}

func callerID(r *http.Request) string {
	p, _ := auth.FromContext(r.Context())
	return p.UserID
}

// GET /schedules/{scheduleID}/availability?date=YYYY-MM-DD
func (h *Handler) Availability(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "scheduleID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	a, err := h.store.Availability(r.Context(), id, r.URL.Query().Get("date"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, a)
}

type bookInput struct {
	ScheduleID     string `json:"schedule_id"`
	Date           string `json:"date"`
	FamilyMemberID string `json:"family_member_id"` // empty: for the account holder
}

// POST /appointments
func (h *Handler) Book(w http.ResponseWriter, r *http.Request) {
	var in bookInput
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if !httpx.IsUUID(in.ScheduleID) {
		h.fail(w, r, apperr.Validation("schedule_id is required"))
		return
	}
	if in.FamilyMemberID != "" && !httpx.IsUUID(in.FamilyMemberID) {
		h.fail(w, r, apperr.Validation("family_member_id must be a UUID"))
		return
	}
	a, err := h.store.BookFor(r.Context(), callerID(r), in.FamilyMemberID, in.ScheduleID, in.Date)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, a)
}

// GET /appointments
func (h *Handler) Mine(w http.ResponseWriter, r *http.Request) {
	limit, offset := httpx.Page(r)
	as, err := h.store.ListForPatient(r.Context(), callerID(r), limit, offset)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"appointments": as, "limit": limit, "offset": offset})
}

// POST /appointments/{appointmentID}/cancel
func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "appointmentID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	a, err := h.store.Cancel(r.Context(), callerID(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, a)
}

// GET /doctor/appointments?date=YYYY-MM-DD (defaults to today)
func (h *Handler) DoctorDay(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if date == "" {
		date = h.store.Today()
	}
	as, err := h.store.ListForDoctor(r.Context(), callerID(r), date)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"date": date, "appointments": as})
}

// POST /doctor/appointments/{appointmentID}/complete
func (h *Handler) Complete(w http.ResponseWriter, r *http.Request) { h.outcome(w, r, StatusCompleted) }

// POST /doctor/appointments/{appointmentID}/no-show
func (h *Handler) NoShow(w http.ResponseWriter, r *http.Request) { h.outcome(w, r, StatusNoShow) }

func (h *Handler) outcome(w http.ResponseWriter, r *http.Request, status string) {
	id, err := httpx.PathUUID(r, "appointmentID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	a, err := h.store.SetOutcome(r.Context(), callerID(r), id, status)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, a)
}

// GET /doctor/tickets/{code}
func (h *Handler) Ticket(w http.ResponseWriter, r *http.Request) {
	a, err := h.store.FindByTicket(r.Context(), callerID(r), chi.URLParam(r, "code"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, a)
}
