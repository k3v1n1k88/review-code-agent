package errors

import "errors"

// Domain sentinel errors used across layers.
var (
	ErrNotFound     = errors.New("not found")
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrConflict     = errors.New("conflict")
	ErrValidation   = errors.New("validation error")
)

// AppError wraps a domain sentinel with optional context message.
type AppError struct {
	Sentinel error
	Message  string
}

func (e *AppError) Error() string {
	if e.Message != "" {
		return e.Sentinel.Error() + ": " + e.Message
	}
	return e.Sentinel.Error()
}

func (e *AppError) Unwrap() error { return e.Sentinel }

// New returns an AppError wrapping the given sentinel with a context message.
func New(sentinel error, msg string) *AppError {
	return &AppError{Sentinel: sentinel, Message: msg}
}
