package httpx

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"healthcare-platform/internal/platform/apperr"
)

var statusByKind = map[apperr.Kind]int{
	apperr.KindValidation: http.StatusUnprocessableEntity,
	apperr.KindNotFound:   http.StatusNotFound,
	apperr.KindConflict:   http.StatusConflict,
	apperr.KindForbidden:  http.StatusForbidden,
}

// WriteError maps domain errors to their HTTP status and hides everything else behind a 500.
func WriteError(w http.ResponseWriter, r *http.Request, log *slog.Logger, err error) {
	if e, ok := apperr.As(err); ok {
		Error(w, statusByKind[e.Kind], e.Msg)
		return
	}
	log.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	Error(w, http.StatusInternalServerError, "internal server error")
}

// IsUUID reports whether s is a canonical 36-character UUID.
func IsUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	_, err := uuid.Parse(s)
	return err == nil
}

// PathUUID reads a UUID path parameter.
func PathUUID(r *http.Request, name string) (string, error) {
	v := chi.URLParam(r, name)
	if !IsUUID(v) {
		return "", apperr.Validation("invalid %s", name)
	}
	return v, nil
}

// Page reads ?limit= and ?offset= with a default limit of 20 and a maximum of 100.
func Page(r *http.Request) (limit, offset int) {
	limit = 20
	q := r.URL.Query()
	if v, err := strconv.Atoi(q.Get("limit")); err == nil && v > 0 {
		limit = min(v, 100)
	}
	if v, err := strconv.Atoi(q.Get("offset")); err == nil && v > 0 {
		offset = v
	}
	return limit, offset
}
