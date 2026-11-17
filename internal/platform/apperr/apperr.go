// Package apperr defines domain errors that map cleanly onto HTTP status codes.
package apperr

import (
	"errors"
	"fmt"
)

type Kind int

const (
	KindValidation Kind = iota + 1 // 422
	KindNotFound                   // 404
	KindConflict                   // 409
	KindForbidden                  // 403
)

// Error is a user-facing error. Msg is safe to return to API clients.
type Error struct {
	Kind Kind
	Msg  string
}

func (e *Error) Error() string { return e.Msg }

func Validation(format string, a ...any) error { return newErr(KindValidation, format, a) }
func NotFound(format string, a ...any) error   { return newErr(KindNotFound, format, a) }
func Conflict(format string, a ...any) error   { return newErr(KindConflict, format, a) }
func Forbidden(format string, a ...any) error  { return newErr(KindForbidden, format, a) }

func newErr(k Kind, format string, a []any) error {
	return &Error{Kind: k, Msg: fmt.Sprintf(format, a...)}
}

// As extracts an *Error from err, if there is one.
func As(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}

// Is reports whether err is an *Error of the given kind.
func Is(err error, k Kind) bool {
	e, ok := As(err)
	return ok && e.Kind == k
}
