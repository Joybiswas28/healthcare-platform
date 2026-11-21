package family

import (
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

// GET /family
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	ms, err := h.store.List(r.Context(), callerID(r))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"members": ms, "max": MaxMembers})
}

// POST /family
func (h *Handler) Add(w http.ResponseWriter, r *http.Request) {
	var in Input
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	m, err := h.store.Add(r.Context(), callerID(r), in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, m)
}

// PUT /family/{memberID}
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "memberID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var in Input
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	m, err := h.store.Update(r.Context(), callerID(r), id, in)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, m)
}

// DELETE /family/{memberID}
func (h *Handler) Remove(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "memberID")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.store.Remove(r.Context(), callerID(r), id); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
