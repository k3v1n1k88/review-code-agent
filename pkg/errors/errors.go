package errors

import (
	"errors"
	"fmt"
)

// Domain error sentinel types.
var (
	ErrNotFound     = errors.New("not found")
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrConflict     = errors.New("conflict")
	ErrValidation   = errors.New("validation error")
	ErrInternal     = errors.New("internal error")
)

// AppError wraps a sentinel with an optional human-readable message.
type AppError struct {
	Kind    error
	Message string
}

func (e *AppError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("%s: %s", e.Kind.Error(), e.Message)
	}
	return e.Kind.Error()
}

func (e *AppError) Unwrap() error { return e.Kind }

// New creates an AppError with the given sentinel kind and message.
func New(kind error, msg string) *AppError {
	return &AppError{Kind: kind, Message: msg}
}

// Wrap annotates err with context, preserving its original type for errors.Is.
func Wrap(err error, msg string) error {
	return fmt.Errorf("%s: %w", msg, err)
}
