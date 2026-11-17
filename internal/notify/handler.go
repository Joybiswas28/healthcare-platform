package notify

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"healthcare-platform/internal/auth"
	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/platform/httpx"
)

type Handler struct {
	store *Store
	log   *slog.Logger
}

func NewHandler(store *Store, log *slog.Logger) *Handler { return &Handler{store: store, log: log} }

func callerID(r *http.Request) string {
	p, _ := auth.FromContext(r.Context())
	return p.UserID
}

// GET /notifications?unread=true
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	limit, offset := httpx.Page(r)
	ns, unread, err := h.store.List(r.Context(), callerID(r), r.URL.Query().Get("unread") == "true", limit, offset)
	if err != nil {
		httpx.WriteError(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"notifications": ns, "unread_count": unread, "limit": limit, "offset": offset})
}

// POST /notifications/{notificationID}/read
func (h *Handler) MarkRead(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "notificationID"), 10, 64)
	if err != nil {
		httpx.WriteError(w, r, h.log, apperr.Validation("invalid notification id"))
		return
	}
	if err := h.store.MarkRead(r.Context(), callerID(r), id); err != nil {
		httpx.WriteError(w, r, h.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /notifications/read-all
func (h *Handler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	if err := h.store.MarkAllRead(r.Context(), callerID(r)); err != nil {
		httpx.WriteError(w, r, h.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
