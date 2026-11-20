package doctor

import (
	"context"
	"log/slog"
	"net/http"

	"healthcare-platform/internal/auth"
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

// ---- public

// GET /doctors?name=&specialization=&location=&limit=&offset=
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, offset := httpx.Page(r)
	res, err := h.store.Search(r.Context(), SearchParams{
		Name:           q.Get("name"),
		Specialization: q.Get("specialization"),
		Location:       q.Get("location"),
		OnlineOnly:     q.Get("online") == "true",
		Sort:           q.Get("sort"),
		Limit:          limit,
		Offset:         offset,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"doctors": res, "limit": limit, "offset": offset})
}

// GET /doctors/{doctorID}
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "doctorID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	d, err := h.store.GetPublic(r.Context(), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, d)
}

// ---- doctor (self)

// GET /doctor/profile
func (h *Handler) MyProfile(w http.ResponseWriter, r *http.Request) {
	p, err := h.store.GetProfile(r.Context(), callerID(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}

// PUT /doctor/profile
func (h *Handler) UpdateMyProfile(w http.ResponseWriter, r *http.Request) {
	var in ProfileInput
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	p, err := h.store.UpdateProfile(r.Context(), callerID(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}

// GET /doctor/chambers
func (h *Handler) MyChambers(w http.ResponseWriter, r *http.Request) {
	cs, err := h.store.ListChambers(r.Context(), callerID(r), false)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"chambers": cs})
}

// POST /doctor/chambers
func (h *Handler) CreateChamber(w http.ResponseWriter, r *http.Request) {
	var in ChamberInput
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	c, err := h.store.CreateChamber(r.Context(), callerID(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, c)
}

// PUT /doctor/chambers/{chamberID}
func (h *Handler) UpdateChamber(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "chamberID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var in ChamberInput
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	c, err := h.store.UpdateChamber(r.Context(), callerID(r), id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, c)
}

// POST /doctor/chambers/{chamberID}/schedules
func (h *Handler) AddSchedule(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "chamberID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var in ScheduleInput
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	sc, err := h.store.AddSchedule(r.Context(), DoctorScope(callerID(r)), id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, sc)
}

// DELETE /doctor/schedules/{scheduleID}
func (h *Handler) RemoveSchedule(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "scheduleID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.store.RemoveSchedule(r.Context(), DoctorScope(callerID(r)), id); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PUT /doctor/consultation-settings
func (h *Handler) UpdateConsultationSettings(w http.ResponseWriter, r *http.Request) {
	var in ConsultationSettingsInput
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	p, err := h.store.SetConsultationSettings(r.Context(), callerID(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}

// ---- admin

// PUT /admin/doctors/{doctorID}/commission
func (h *Handler) AdminSetCommission(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "doctorID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var in CommissionInput
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	p, err := h.store.SetCommission(r.Context(), id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}

// GET /admin/doctors?status=pending
func (h *Handler) AdminList(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	switch status {
	case "", StatusPending, StatusApproved, StatusRejected:
	default:
		httpx.Error(w, http.StatusUnprocessableEntity, "status must be pending, approved or rejected")
		return
	}
	limit, offset := httpx.Page(r)
	ds, err := h.store.ListForAdmin(r.Context(), status, limit, offset)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"doctors": ds, "limit": limit, "offset": offset})
}

// POST /admin/doctors/{doctorID}/verification
func (h *Handler) AdminVerify(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "doctorID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var in VerifyInput
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	p, err := h.store.Verify(r.Context(), id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}

// ---- reviews

// POST /appointments/{appointmentID}/review
func (h *Handler) ReviewAppointment(w http.ResponseWriter, r *http.Request) {
	h.addReview(w, r, "appointmentID", h.store.ReviewAppointment)
}

// POST /consultations/{consultationID}/review
func (h *Handler) ReviewConsultation(w http.ResponseWriter, r *http.Request) {
	h.addReview(w, r, "consultationID", h.store.ReviewConsultation)
}

func (h *Handler) addReview(w http.ResponseWriter, r *http.Request, param string,
	add func(ctx context.Context, patientID, visitID string, in ReviewInput) (Review, error)) {
	id, err := httpx.PathUUID(r, param)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var in ReviewInput
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	v, err := add(r.Context(), callerID(r), id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, v)
}

// GET /reviews/mine
func (h *Handler) MyReviews(w http.ResponseWriter, r *http.Request) {
	vs, err := h.store.MyReviews(r.Context(), callerID(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"reviews": vs})
}

// GET /doctor/reviews
func (h *Handler) DoctorReviews(w http.ResponseWriter, r *http.Request) {
	limit, offset := httpx.Page(r)
	p, err := h.store.GetProfile(r.Context(), callerID(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	vs, err := h.store.DoctorReviews(r.Context(), callerID(r), limit, offset)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"rating": p.Rating, "review_count": p.ReviewCount, "reviews": vs, "limit": limit, "offset": offset,
	})
}

// PUT /doctor/reviews/{reviewID}/reply
func (h *Handler) ReplyToReview(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "reviewID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var in ReplyInput
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	v, err := h.store.Reply(r.Context(), callerID(r), id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, v)
}
