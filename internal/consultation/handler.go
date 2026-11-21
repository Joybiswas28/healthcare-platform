package consultation

import (
	"bufio"
	"context"
	"log/slog"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"healthcare-platform/internal/auth"
	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/platform/httpx"
	"healthcare-platform/internal/realtime"
	"healthcare-platform/internal/user"
)

// MaxUploadBytes is the largest file a user may upload (10 MB).
const MaxUploadBytes = 10 << 20

// Reports and prescriptions are accepted as PDF or images only. The type is
// detected from the file's content, not from the name or the client's header.
var allowedTypes = map[string]bool{
	"application/pdf": true,
	"image/jpeg":      true,
	"image/png":       true,
}

type Handler struct {
	store *Store
	hub   *realtime.Hub
	log   *slog.Logger
}

func NewHandler(store *Store, hub *realtime.Hub, log *slog.Logger) *Handler {
	return &Handler{store: store, hub: hub, log: log}
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	httpx.WriteError(w, r, h.log, err)
}

func caller(r *http.Request) auth.Principal {
	p, _ := auth.FromContext(r.Context())
	return p
}

// POST /consultations (patient)
func (h *Handler) Request(w http.ResponseWriter, r *http.Request) {
	var in RequestInput
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if !httpx.IsUUID(in.DoctorID) {
		h.fail(w, r, apperr.Validation("doctor_id is required"))
		return
	}
	if in.FamilyMemberID != "" && !httpx.IsUUID(in.FamilyMemberID) {
		h.fail(w, r, apperr.Validation("family_member_id must be a UUID"))
		return
	}
	c, err := h.store.Request(r.Context(), caller(r).UserID, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, c)
}

// GET /consultations?status=
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	p := caller(r)
	limit, offset := httpx.Page(r)
	cs, err := h.store.List(r.Context(), p.UserID, p.Role, r.URL.Query().Get("status"), limit, offset)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"consultations": cs, "limit": limit, "offset": offset})
}

type action func(ctx context.Context, userID, id string) (Consultation, error)

func (h *Handler) run(fn action) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := httpx.PathUUID(r, "consultationID")
		if err != nil {
			h.fail(w, r, err)
			return
		}
		c, err := fn(r.Context(), caller(r).UserID, id)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, c)
	}
}

// GET /consultations/{consultationID}
func (h *Handler) Get() http.HandlerFunc { return h.run(h.store.Get) }

// POST /consultations/{consultationID}/accept (doctor)
func (h *Handler) Accept() http.HandlerFunc { return h.run(h.store.Accept) }

// POST /consultations/{consultationID}/confirm (patient): pays with tokens
func (h *Handler) Confirm() http.HandlerFunc { return h.run(h.store.Confirm) }

// POST /consultations/{consultationID}/complete (doctor)
func (h *Handler) Complete() http.HandlerFunc { return h.run(h.store.Complete) }

// POST /consultations/{consultationID}/cancel
func (h *Handler) Cancel() http.HandlerFunc { return h.run(h.store.Cancel) }

type rejectInput struct {
	Reason string `json:"reason"`
}

// POST /consultations/{consultationID}/reject (doctor)
func (h *Handler) Reject(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "consultationID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var in rejectInput
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	c, err := h.store.Reject(r.Context(), caller(r).UserID, id, in.Reason)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, c)
}

// GET /consultations/{consultationID}/messages?after=<last seen id>
func (h *Handler) Messages(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "consultationID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	limit, _ := httpx.Page(r)
	ms, err := h.store.Messages(r.Context(), caller(r).UserID, id, max(after, 0), limit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"messages": ms})
}

type messageInput struct {
	Body string `json:"body"`
}

// POST /consultations/{consultationID}/messages
func (h *Handler) PostMessage(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "consultationID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var in messageInput
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	m, err := h.store.PostMessage(r.Context(), caller(r).UserID, id, in.Body)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, m)
}

// POST /consultations/{consultationID}/attachments (multipart form, field "file")
func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "consultationID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxUploadBytes+(1<<20)) // room for multipart headers
	mr, err := r.MultipartReader()
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "expected a multipart/form-data upload")
		return
	}

	for {
		part, err := mr.NextPart()
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, `missing "file" field (or upload larger than 10 MB)`)
			return
		}
		if part.FormName() != "file" {
			continue
		}

		name := cleanFileName(part.FileName())
		br := bufio.NewReaderSize(part, 512)
		head, _ := br.Peek(512)
		contentType, _, _ := mime.ParseMediaType(http.DetectContentType(head))
		if !allowedTypes[contentType] {
			httpx.Error(w, http.StatusUnsupportedMediaType, "only PDF, JPEG and PNG files are allowed")
			return
		}

		limited := &limitReader{r: br, left: MaxUploadBytes}
		m, err := h.store.Upload(r.Context(), caller(r).UserID, id, name, contentType, limited)
		if limited.exceeded {
			httpx.Error(w, http.StatusRequestEntityTooLarge, "file is larger than 10 MB")
			return
		}
		if err != nil {
			h.fail(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, m)
		return
	}
}

// GET /attachments/{attachmentID}
func (h *Handler) Download(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "attachmentID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	a, rc, err := h.store.OpenAttachment(r.Context(), caller(r).UserID, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", a.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(a.SizeBytes, 10))
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": a.FileName}))
	// User-uploaded content: never let it run scripts in our origin.
	w.Header().Set("Content-Security-Policy", "sandbox")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
	if _, err := bufio.NewReader(rc).WriteTo(w); err != nil {
		h.log.WarnContext(r.Context(), "download interrupted", "attachment", id, "err", err)
	}
}

// ---------------------------------------------------------------- public concerns

// POST /concerns (patient)
func (h *Handler) PostConcern(w http.ResponseWriter, r *http.Request) {
	var in ConcernInput
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	c, err := h.store.PostConcern(r.Context(), caller(r).UserID, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, c)
}

// GET /concerns: patients get their own concerns; doctors get the open feed
// (?specialization= filters it).
func (h *Handler) Concerns(w http.ResponseWriter, r *http.Request) {
	p := caller(r)
	limit, offset := httpx.Page(r)
	var cs []Concern
	var err error
	if p.Role == user.RoleDoctor {
		cs, err = h.store.OpenConcerns(r.Context(), r.URL.Query().Get("specialization"), limit, offset)
	} else {
		cs, err = h.store.MyConcerns(r.Context(), p.UserID, limit, offset)
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"concerns": cs, "limit": limit, "offset": offset})
}

// GET /concerns/{concernID}: the patient sees all offers; a doctor sees only their own.
func (h *Handler) Concern(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "concernID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	p := caller(r)
	var c Concern
	if p.Role == user.RoleDoctor {
		c, err = h.store.DoctorConcern(r.Context(), p.UserID, id)
	} else {
		c, err = h.store.PatientConcern(r.Context(), p.UserID, id)
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, c)
}

// POST /concerns/{concernID}/close (patient)
func (h *Handler) CloseConcern(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "concernID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	c, err := h.store.CloseConcern(r.Context(), caller(r).UserID, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, c)
}

type offerInput struct {
	Message string `json:"message"`
}

// POST /concerns/{concernID}/offers (doctor)
func (h *Handler) MakeOffer(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "concernID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var in offerInput
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	o, err := h.store.MakeOffer(r.Context(), caller(r).UserID, id, in.Message)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, o)
}

// GET /doctor/offers
func (h *Handler) MyOffers(w http.ResponseWriter, r *http.Request) {
	limit, offset := httpx.Page(r)
	offers, err := h.store.MyOffers(r.Context(), caller(r).UserID, limit, offset)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"offers": offers, "limit": limit, "offset": offset})
}

// POST /offers/{offerID}/withdraw (doctor)
func (h *Handler) WithdrawOffer(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "offerID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	o, err := h.store.WithdrawOffer(r.Context(), caller(r).UserID, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, o)
}

// POST /offers/{offerID}/accept (patient): pays with tokens and starts the consultation
func (h *Handler) AcceptOffer(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "offerID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	c, err := h.store.AcceptOffer(r.Context(), caller(r).UserID, id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, c)
}

// cleanFileName keeps only the base name and limits its length.
func cleanFileName(name string) string {
	name = filepath.Base(strings.ReplaceAll(name, `\`, "/"))
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, name)
	if name == "." || name == "/" || name == "" {
		name = "file"
	}
	for utf8.RuneCountInString(name) > 120 {
		_, size := utf8.DecodeLastRuneInString(name)
		name = name[:len(name)-size]
	}
	return name
}

// limitReader fails once more than left bytes have been read.
type limitReader struct {
	r        interface{ Read([]byte) (int, error) }
	left     int64
	exceeded bool
}

func (l *limitReader) Read(p []byte) (int, error) {
	n, err := l.r.Read(p)
	l.left -= int64(n)
	if l.left < 0 {
		l.exceeded = true
		return n, errFileTooLarge
	}
	return n, err
}

var errFileTooLarge = apperr.Validation("file is larger than 10 MB")
