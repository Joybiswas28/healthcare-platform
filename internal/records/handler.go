package records

import (
	"bufio"
	"errors"
	"io"
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
)

var allowedTypes = map[string]bool{"application/pdf": true, "image/jpeg": true, "image/png": true}

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

// GET /records?person=self|{memberID}
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	rs, err := h.store.List(r.Context(), callerID(r), r.URL.Query().Get("person"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"records": rs, "max": MaxRecords})
}

// POST /records (multipart: the text fields first, then "file")
func (h *Handler) Add(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxFileBytes+(1<<20)) // room for the other fields
	mr, err := r.MultipartReader()
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "expected a multipart/form-data upload")
		return
	}
	var m Meta
	fields := map[string]*string{
		"title": &m.Title, "kind": &m.Kind, "record_date": &m.RecordDate, "note": &m.Note, "family_member_id": &m.FamilyMemberID,
	}
	for {
		part, err := mr.NextPart()
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, `missing "file" field (or upload larger than 10 MB)`)
			return
		}
		if dst, ok := fields[part.FormName()]; ok {
			b, err := io.ReadAll(io.LimitReader(part, 4096))
			if err != nil {
				httpx.Error(w, http.StatusBadRequest, "could not read the form")
				return
			}
			*dst = string(b)
			continue
		}
		if part.FormName() != "file" {
			continue
		}

		br := bufio.NewReaderSize(part, 512)
		head, _ := br.Peek(512)
		contentType, _, _ := mime.ParseMediaType(http.DetectContentType(head))
		if !allowedTypes[contentType] {
			httpx.Error(w, http.StatusUnsupportedMediaType, "only PDF, JPEG and PNG files are allowed")
			return
		}
		limited := &limitReader{r: br, left: MaxFileBytes}
		rec, err := h.store.Add(r.Context(), callerID(r), m, cleanFileName(part.FileName()), contentType, limited)
		if limited.exceeded {
			httpx.Error(w, http.StatusRequestEntityTooLarge, "file is larger than 10 MB")
			return
		}
		if err != nil {
			h.fail(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusCreated, rec)
		return
	}
}

// PUT /records/{recordID}
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "recordID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var m Meta
	if err := httpx.Decode(w, r, &m); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	rec, err := h.store.Update(r.Context(), callerID(r), id, m)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, rec)
}

// DELETE /records/{recordID}
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "recordID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.store.Delete(r.Context(), callerID(r), id); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /records/{recordID}/file (the owner, or a doctor it was shared with)
func (h *Handler) File(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "recordID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	f, rc, err := h.store.Open(r.Context(), callerID(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	defer rc.Close()
	disposition := "attachment"
	if r.URL.Query().Get("inline") == "1" {
		disposition = "inline"
	}
	w.Header().Set("Content-Type", f.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(f.SizeBytes, 10))
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": f.FileName}))
	// User-uploaded content: never let it run scripts in our origin.
	w.Header().Set("Content-Security-Policy", "sandbox")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
	if _, err := bufio.NewReader(rc).WriteTo(w); err != nil {
		h.log.WarnContext(r.Context(), "download interrupted", "record", id, "err", err)
	}
}

type shareInput struct {
	AppointmentID  string `json:"appointment_id"`
	ConsultationID string `json:"consultation_id"`
}

// POST /records/{recordID}/share {appointment_id} or {consultation_id}
func (h *Handler) Share(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "recordID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var in shareInput
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	switch {
	case httpx.IsUUID(in.AppointmentID) && in.ConsultationID == "":
		rec, err := h.store.ShareWithAppointment(r.Context(), callerID(r), id, in.AppointmentID)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"record": rec})
	case httpx.IsUUID(in.ConsultationID) && in.AppointmentID == "":
		msgID, err := h.store.SendToConsultation(r.Context(), callerID(r), id, in.ConsultationID)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"message_id": msgID})
	default:
		h.fail(w, r, apperr.Validation("give either appointment_id or consultation_id"))
	}
}

// DELETE /records/{recordID}/shares/{appointmentID}
func (h *Handler) Unshare(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "recordID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	appointmentID, err := httpx.PathUUID(r, "appointmentID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	rec, err := h.store.Unshare(r.Context(), callerID(r), id, appointmentID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, rec)
}

// GET /doctor/appointments/{appointmentID}/records
func (h *Handler) ForAppointment(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "appointmentID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	rs, err := h.store.ForAppointment(r.Context(), callerID(r), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"records": rs})
}

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
	r        io.Reader
	left     int64
	exceeded bool
}

var errFileTooLarge = errors.New("file is larger than 10 MB")

func (l *limitReader) Read(p []byte) (int, error) {
	n, err := l.r.Read(p)
	l.left -= int64(n)
	if l.left < 0 {
		l.exceeded = true
		return n, errFileTooLarge
	}
	return n, err
}
