package prescription

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"

	"healthcare-platform/internal/auth"
	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/platform/httpx"
	"healthcare-platform/internal/user"
)

type Handler struct {
	store *Store
	log   *slog.Logger
	// frontendURL is the website; the QR code on a printed prescription opens its
	// /verify/{code} page, which pharmacies can read (the API answer is raw JSON).
	frontendURL string
}

func NewHandler(store *Store, frontendURL string, log *slog.Logger) *Handler {
	return &Handler{store: store, frontendURL: strings.TrimRight(frontendURL, "/"), log: log}
}

func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	httpx.WriteError(w, r, h.log, err)
}

func (h *Handler) verifyURL(code string) string {
	return h.frontendURL + "/verify/" + url.PathEscape(code)
}

// POST /prescriptions (doctor)
func (h *Handler) Issue(w http.ResponseWriter, r *http.Request) {
	var in IssueInput
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	for _, id := range []string{in.ConsultationID, in.AppointmentID} {
		if id != "" && !httpx.IsUUID(strings.TrimSpace(id)) {
			h.fail(w, r, apperr.Validation("consultation_id / appointment_id must be a valid id"))
			return
		}
	}
	p, _ := auth.FromContext(r.Context())
	rx, err := h.store.Issue(r.Context(), p.UserID, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, rx)
}

// GET /prescriptions?consultation_id=&appointment_id=
// Patients see prescriptions issued to them; doctors see the ones they issued.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	p, _ := auth.FromContext(r.Context())
	q := r.URL.Query()
	cid, aid := q.Get("consultation_id"), q.Get("appointment_id")
	if (cid != "" && !httpx.IsUUID(cid)) || (aid != "" && !httpx.IsUUID(aid)) {
		h.fail(w, r, apperr.Validation("invalid consultation_id or appointment_id"))
		return
	}
	limit, offset := httpx.Page(r)
	list, err := h.store.List(r.Context(), p.UserID, p.Role == user.RoleDoctor, cid, aid, limit, offset)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"prescriptions": list, "limit": limit, "offset": offset})
}

// GET /prescriptions/{prescriptionID}
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	rx, ok := h.load(w, r)
	if ok {
		httpx.JSON(w, http.StatusOK, rx)
	}
}

// GET /prescriptions/{prescriptionID}/print returns a printable HTML page.
func (h *Handler) Print(w http.ResponseWriter, r *http.Request) {
	rx, ok := h.load(w, r)
	if !ok {
		return
	}
	html, err := RenderString(rx, h.verifyURL(rx.Code))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src data:; style-src 'unsafe-inline'")
	_, _ = w.Write([]byte(html))
}

func (h *Handler) load(w http.ResponseWriter, r *http.Request) (Prescription, bool) {
	id, err := httpx.PathUUID(r, "prescriptionID")
	if err != nil {
		h.fail(w, r, err)
		return Prescription{}, false
	}
	p, _ := auth.FromContext(r.Context())
	rx, err := h.store.Get(r.Context(), p.UserID, id)
	if err != nil {
		h.fail(w, r, err)
		return Prescription{}, false
	}
	return rx, true
}

type revokeInput struct {
	Reason string `json:"reason"`
}

// POST /prescriptions/{prescriptionID}/revoke (doctor)
func (h *Handler) Revoke(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "prescriptionID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var in revokeInput
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	p, _ := auth.FromContext(r.Context())
	rx, err := h.store.Revoke(r.Context(), p.UserID, id, in.Reason)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, rx)
}

// GET /prescriptions/verify/{code} (public)
func (h *Handler) Verify(w http.ResponseWriter, r *http.Request) {
	v, err := h.store.Verify(r.Context(), chi.URLParam(r, "code"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, v)
}
