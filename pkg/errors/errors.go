package errors

import (
	"errors"
	"fmt"
)

// Domain error types for the application.

type NotFoundError struct{ Resource string }

func (e *NotFoundError) Error() string { return fmt.Sprintf("%s not found", e.Resource) }

type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }

type UnauthorizedError struct{ Message string }

func (e *UnauthorizedError) Error() string {
	if e.Message == "" {
		return "unauthorized"
	}
	return e.Message
}

type ConflictError struct{ Message string }

func (e *ConflictError) Error() string { return e.Message }

// IsNotFound returns true when err (or any wrapped error) is a *NotFoundError.
func IsNotFound(err error) bool      { var e *NotFoundError; return errors.As(err, &e) }
func IsValidation(err error) bool    { var e *ValidationError; return errors.As(err, &e) }
func IsUnauthorized(err error) bool  { var e *UnauthorizedError; return errors.As(err, &e) }
func IsConflict(err error) bool      { var e *ConflictError; return errors.As(err, &e) }
