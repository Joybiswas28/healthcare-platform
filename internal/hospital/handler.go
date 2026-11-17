package hospital

import (
	"bufio"
	"context"
	"log/slog"
	"mime"
	"net/http"
	"strconv"

	"healthcare-platform/internal/appointment"
	"healthcare-platform/internal/auth"
	"healthcare-platform/internal/doctor"
	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/platform/httpx"
)

type Handler struct {
	store        *Store
	doctors      *doctor.Store
	appointments *appointment.Store
	log          *slog.Logger
}

func NewHandler(store *Store, doctors *doctor.Store, appointments *appointment.Store, log *slog.Logger) *Handler {
	return &Handler{store: store, doctors: doctors, appointments: appointments, log: log}
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	httpx.WriteError(w, r, h.log, err)
}

func callerID(r *http.Request) string {
	p, _ := auth.FromContext(r.Context())
	return p.UserID
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := httpx.Decode(w, r, dst); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return false
	}
	return true
}

// ---------------------------------------------------------------- access control

type accessKey struct{}

type access struct {
	hospitalID string
	role       string
}

// Member lets the request through only if the caller manages the hospital in the URL.
func (h *Handler) Member(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := httpx.PathUUID(r, "hospitalID")
		if err != nil {
			h.fail(w, r, err)
			return
		}
		role, err := h.store.Role(r.Context(), callerID(r), id)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		ctx := context.WithValue(r.Context(), accessKey{}, access{hospitalID: id, role: role})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// OwnerOnly must run after Member.
func (h *Handler) OwnerOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if acc(r).role != RoleOwner {
			h.fail(w, r, apperr.Forbidden("only the hospital owner can do this"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func acc(r *http.Request) access {
	a, _ := r.Context().Value(accessKey{}).(access)
	return a
}

func hid(r *http.Request) string { return acc(r).hospitalID }

// ---------------------------------------------------------------- public

// GET /hospitals?name=&location=&department=&facility=
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, offset := httpx.Page(r)
	list, err := h.store.Search(r.Context(), SearchParams{
		Name: q.Get("name"), Location: q.Get("location"), Department: q.Get("department"),
		Facility: q.Get("facility"), Limit: limit, Offset: offset,
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"hospitals": list, "limit": limit, "offset": offset})
}

// GET /hospitals/{hospitalID}
func (h *Handler) Public(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "hospitalID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	p, err := h.store.Public(r.Context(), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}

// GET /hospitals/{hospitalID}/photos/{photoID}
func (h *Handler) Photo(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "hospitalID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	pid, err := httpx.PathUUID(r, "photoID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	ct, size, rc, err := h.store.OpenPhoto(r.Context(), id, pid)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = bufio.NewReader(rc).WriteTo(w)
}

// ---------------------------------------------------------------- hospital admin

// POST /hospitals (hospital_admin)
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var in HospitalInput
	if !decode(w, r, &in) {
		return
	}
	hosp, err := h.store.Create(r.Context(), callerID(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, hosp)
}

// GET /manage/hospitals
func (h *Handler) Mine(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListMine(r.Context(), callerID(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"hospitals": list})
}

// GET /manage/hospitals/{hospitalID}
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	hosp, err := h.store.GetForMember(r.Context(), callerID(r), hid(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, hosp)
}

// PUT /manage/hospitals/{hospitalID}
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	var in HospitalInput
	if !decode(w, r, &in) {
		return
	}
	hosp, err := h.store.Update(r.Context(), callerID(r), hid(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, hosp)
}

// POST /manage/hospitals/{hospitalID}/photos (multipart, field "file")
func (h *Handler) UploadPhoto(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxPhotoBytes+(1<<20))
	mr, err := r.MultipartReader()
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "expected a multipart/form-data upload")
		return
	}
	for {
		part, err := mr.NextPart()
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, `missing "file" field (or photo larger than 5 MB)`)
			return
		}
		if part.FormName() != "file" {
			continue
		}
		br := bufio.NewReaderSize(part, 512)
		head, _ := br.Peek(512)
		ct, _, _ := mime.ParseMediaType(http.DetectContentType(head))
		if !PhotoTypes[ct] {
			httpx.Error(w, http.StatusUnsupportedMediaType, "photos must be JPEG, PNG or WebP")
			return
		}
		limited := &limitReader{r: br, left: MaxPhotoBytes}
		p, err := h.store.AddPhoto(r.Context(), hid(r), ct, limited)
		if limited.exceeded {
			httpx.Error(w, http.StatusRequestEntityTooLarge, "photo is larger than 5 MB")
			return
		}
		if err != nil {
			h.fail(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, p)
		return
	}
}

type limitReader struct {
	r        *bufio.Reader
	left     int64
	exceeded bool
}

func (l *limitReader) Read(p []byte) (int, error) {
	n, err := l.r.Read(p)
	l.left -= int64(n)
	if l.left < 0 {
		l.exceeded = true
		return n, apperr.Validation("photo is larger than 5 MB")
	}
	return n, err
}

// DELETE /manage/hospitals/{hospitalID}/photos/{photoID}
func (h *Handler) DeletePhoto(w http.ResponseWriter, r *http.Request) {
	pid, err := httpx.PathUUID(r, "photoID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.store.DeletePhoto(r.Context(), hid(r), pid); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /manage/hospitals/{hospitalID}/photos/{photoID}/logo
func (h *Handler) SetLogo(w http.ResponseWriter, r *http.Request) {
	pid, err := httpx.PathUUID(r, "photoID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.store.SetLogo(r.Context(), hid(r), pid); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- staff doctors

// GET /manage/hospitals/{hospitalID}/doctors
func (h *Handler) Staff(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.Staff(r.Context(), hid(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"doctors": list})
}

// POST /manage/hospitals/{hospitalID}/doctors
func (h *Handler) Invite(w http.ResponseWriter, r *http.Request) {
	var in InviteInput
	if !decode(w, r, &in) {
		return
	}
	sd, err := h.store.Invite(r.Context(), hid(r), callerID(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, sd)
}

// PUT /manage/hospitals/{hospitalID}/doctors/{doctorID}
func (h *Handler) UpdateStaff(w http.ResponseWriter, r *http.Request) {
	did, err := httpx.PathUUID(r, "doctorID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var in StaffInput
	if !decode(w, r, &in) {
		return
	}
	sd, err := h.store.UpdateStaff(r.Context(), hid(r), did, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, sd)
}

// DELETE /manage/hospitals/{hospitalID}/doctors/{doctorID}
func (h *Handler) RemoveStaff(w http.ResponseWriter, r *http.Request) {
	did, err := httpx.PathUUID(r, "doctorID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	upcoming, err := h.store.RemoveStaff(r.Context(), hid(r), did)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"status":                     DoctorRemoved,
		"upcoming_appointments":      upcoming,
		"upcoming_appointments_note": "booking is now off for this doctor; cancel or reschedule any upcoming appointments",
	})
}

// ---- chambers and schedules

// GET /manage/hospitals/{hospitalID}/chambers?doctor_id=
func (h *Handler) Chambers(w http.ResponseWriter, r *http.Request) {
	did := r.URL.Query().Get("doctor_id")
	if did != "" && !httpx.IsUUID(did) {
		h.fail(w, r, apperr.Validation("invalid doctor_id"))
		return
	}
	cs, err := h.doctors.ListHospitalChambers(r.Context(), hid(r), did, false)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"chambers": cs})
}

// POST /manage/hospitals/{hospitalID}/doctors/{doctorID}/chambers
func (h *Handler) CreateChamber(w http.ResponseWriter, r *http.Request) {
	did, err := httpx.PathUUID(r, "doctorID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var in ChamberInput
	if !decode(w, r, &in) {
		return
	}
	c, err := h.store.CreateChamber(r.Context(), hid(r), did, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, c)
}

// PUT /manage/hospitals/{hospitalID}/chambers/{chamberID}
func (h *Handler) UpdateChamber(w http.ResponseWriter, r *http.Request) {
	cid, err := httpx.PathUUID(r, "chamberID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var in ChamberInput
	if !decode(w, r, &in) {
		return
	}
	c, err := h.store.UpdateChamber(r.Context(), hid(r), cid, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, c)
}

// POST /manage/hospitals/{hospitalID}/chambers/{chamberID}/schedules
func (h *Handler) AddSchedule(w http.ResponseWriter, r *http.Request) {
	cid, err := httpx.PathUUID(r, "chamberID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var in doctor.ScheduleInput
	if !decode(w, r, &in) {
		return
	}
	sc, err := h.doctors.AddSchedule(r.Context(), doctor.HospitalScope(hid(r)), cid, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, sc)
}

// DELETE /manage/hospitals/{hospitalID}/schedules/{scheduleID}
func (h *Handler) RemoveSchedule(w http.ResponseWriter, r *http.Request) {
	sid, err := httpx.PathUUID(r, "scheduleID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.doctors.RemoveSchedule(r.Context(), doctor.HospitalScope(hid(r)), sid); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- bookings

// GET /manage/hospitals/{hospitalID}/appointments?date=&doctor_id=
func (h *Handler) Appointments(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	date, did := q.Get("date"), q.Get("doctor_id")
	if date == "" {
		date = h.appointments.Today()
	}
	if did != "" && !httpx.IsUUID(did) {
		h.fail(w, r, apperr.Validation("invalid doctor_id"))
		return
	}
	list, err := h.appointments.ListForHospital(r.Context(), hid(r), did, date)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"date": date, "appointments": list})
}

type cancelInput struct {
	Reason string `json:"reason"`
}

// POST /manage/hospitals/{hospitalID}/appointments/{appointmentID}/cancel
func (h *Handler) CancelAppointment(w http.ResponseWriter, r *http.Request) {
	aid, err := httpx.PathUUID(r, "appointmentID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var in cancelInput
	if !decode(w, r, &in) {
		return
	}
	a, err := h.appointments.CancelByHospital(r.Context(), hid(r), aid, in.Reason)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, a)
}

type rescheduleInput struct {
	ScheduleID string `json:"schedule_id"`
	Date       string `json:"date"`
}

// POST /manage/hospitals/{hospitalID}/appointments/{appointmentID}/reschedule
func (h *Handler) RescheduleAppointment(w http.ResponseWriter, r *http.Request) {
	aid, err := httpx.PathUUID(r, "appointmentID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var in rescheduleInput
	if !decode(w, r, &in) {
		return
	}
	if !httpx.IsUUID(in.ScheduleID) {
		h.fail(w, r, apperr.Validation("schedule_id is required"))
		return
	}
	a, err := h.appointments.Reschedule(r.Context(), hid(r), aid, in.ScheduleID, in.Date)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, a)
}

// ---- members

// GET /manage/hospitals/{hospitalID}/members
func (h *Handler) Members(w http.ResponseWriter, r *http.Request) {
	ms, err := h.store.Members(r.Context(), hid(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"members": ms})
}

type memberInput struct {
	Phone string `json:"phone"`
}

// POST /manage/hospitals/{hospitalID}/members (owner)
func (h *Handler) AddMember(w http.ResponseWriter, r *http.Request) {
	var in memberInput
	if !decode(w, r, &in) {
		return
	}
	phone, err := auth.NormalizePhone(in.Phone)
	if err != nil {
		h.fail(w, r, apperr.Validation("%s", err.Error()))
		return
	}
	m, err := h.store.AddMember(r.Context(), hid(r), phone)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, m)
}

// DELETE /manage/hospitals/{hospitalID}/members/{userID} (owner)
func (h *Handler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	uid, err := httpx.PathUUID(r, "userID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.store.RemoveMember(r.Context(), hid(r), uid); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------- doctor side

// GET /doctor/hospitals
func (h *Handler) Invitations(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.Invitations(r.Context(), callerID(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"hospitals": list})
}

func (h *Handler) respond(accept bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := httpx.PathUUID(r, "hospitalID")
		if err != nil {
			h.fail(w, r, err)
			return
		}
		if err := h.store.Respond(r.Context(), callerID(r), id, accept); err != nil {
			h.fail(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// POST /doctor/hospitals/{hospitalID}/accept
func (h *Handler) AcceptInvitation() http.HandlerFunc { return h.respond(true) }

// POST /doctor/hospitals/{hospitalID}/decline
func (h *Handler) DeclineInvitation() http.HandlerFunc { return h.respond(false) }

// POST /doctor/hospitals/{hospitalID}/leave
func (h *Handler) Leave(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "hospitalID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	upcoming, err := h.store.Leave(r.Context(), callerID(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"status": DoctorLeft, "upcoming_appointments": upcoming})
}

// ---------------------------------------------------------------- platform admin

// GET /admin/hospitals?status=pending
func (h *Handler) AdminList(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	switch status {
	case "", StatusPending, StatusApproved, StatusRejected:
	default:
		h.fail(w, r, apperr.Validation("status must be pending, approved or rejected"))
		return
	}
	limit, offset := httpx.Page(r)
	list, err := h.store.AdminList(r.Context(), status, limit, offset)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"hospitals": list, "limit": limit, "offset": offset})
}

// POST /admin/hospitals/{hospitalID}/verification
func (h *Handler) AdminVerify(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "hospitalID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var in VerifyInput
	if !decode(w, r, &in) {
		return
	}
	hosp, err := h.store.Verify(r.Context(), id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, hosp)
}
