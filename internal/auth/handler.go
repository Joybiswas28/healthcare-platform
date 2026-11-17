package auth

import (
	"errors"
	"log/slog"
	"net/http"

	"healthcare-platform/internal/platform/httpx"
	"healthcare-platform/internal/user"
)

type Handler struct {
	svc *Service
	log *slog.Logger
}

func NewHandler(svc *Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var in RegisterInput
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := h.svc.Register(r.Context(), in)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, res)
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var in LoginInput
	if err := httpx.Decode(w, r, &in); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := h.svc.Login(r.Context(), in)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	p, _ := FromContext(r.Context())
	u, err := h.svc.Me(r.Context(), p.UserID)
	if err != nil {
		h.writeErr(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, u)
}

func (h *Handler) writeErr(w http.ResponseWriter, r *http.Request, err error) {
	var ve ValidationError
	switch {
	case errors.As(err, &ve):
		httpx.Error(w, http.StatusUnprocessableEntity, ve.Msg)
	case errors.Is(err, ErrInvalidCredentials):
		httpx.Error(w, http.StatusUnauthorized, err.Error())
	case errors.Is(err, user.ErrPhoneTaken):
		httpx.Error(w, http.StatusConflict, err.Error())
	case errors.Is(err, user.ErrNotFound):
		httpx.Error(w, http.StatusNotFound, err.Error())
	default:
		h.log.ErrorContext(r.Context(), "request failed", "path", r.URL.Path, "err", err)
		httpx.Error(w, http.StatusInternalServerError, "internal server error")
	}
}
