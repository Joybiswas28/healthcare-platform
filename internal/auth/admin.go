package auth

import (
	"errors"
	"log/slog"
	"net/http"

	"healthcare-platform/internal/platform/apperr"
	"healthcare-platform/internal/platform/httpx"
	"healthcare-platform/internal/user"
)

// AdminHandler lets platform admins suspend or reactivate accounts.
type AdminHandler struct {
	users    *user.Repository
	sessions *SessionChecker
	log      *slog.Logger
}

func NewAdminHandler(users *user.Repository, sessions *SessionChecker, log *slog.Logger) *AdminHandler {
	return &AdminHandler{users: users, sessions: sessions, log: log}
}

var userRoles = map[string]bool{
	"": true, "patient": true, "doctor": true, "hospital_admin": true, "pharmacy": true, "rider": true, "admin": true,
}

// GET /admin/users?q=&role=
// q matches part of the name or phone number.
func (h *AdminHandler) Search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	role := q.Get("role")
	if !userRoles[role] {
		httpx.WriteError(w, r, h.log, apperr.Validation("unknown role"))
		return
	}
	limit, offset := httpx.Page(r)
	list, err := h.users.Search(r.Context(), q.Get("q"), role, limit, offset)
	if err != nil {
		httpx.WriteError(w, r, h.log, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"users": list, "limit": limit, "offset": offset})
}

type statusInput struct {
	Status string `json:"status"` // active | suspended
}

// POST /admin/users/{userID}/status
// Suspending signs the user out everywhere (within SessionChecker's cache time on
// other servers). Admin accounts cannot be suspended here.
func (h *AdminHandler) SetStatus(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathUUID(r, "userID")
	if err != nil {
		httpx.WriteError(w, r, h.log, err)
		return
	}
	var in statusInput
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if in.Status != "active" && in.Status != "suspended" {
		httpx.WriteError(w, r, h.log, apperr.Validation("status must be active or suspended"))
		return
	}
	u, err := h.users.SetStatus(r.Context(), id, in.Status)
	if errors.Is(err, user.ErrNotFound) {
		httpx.WriteError(w, r, h.log, apperr.NotFound("user not found (admin accounts cannot be changed here)"))
		return
	}
	if err != nil {
		httpx.WriteError(w, r, h.log, err)
		return
	}
	h.sessions.Forget(id)
	h.log.InfoContext(r.Context(), "account status changed", "user", id, "status", in.Status)
	httpx.JSON(w, http.StatusOK, u)
}
