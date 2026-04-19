package errors

import "fmt"

type NotFoundError struct{ Resource string }
type ValidationError struct{ Message string }
type UnauthorizedError struct{}
type ConflictError struct{ Message string }

func (e *NotFoundError) Error() string    { return fmt.Sprintf("%s not found", e.Resource) }
func (e *ValidationError) Error() string  { return e.Message }
func (e *UnauthorizedError) Error() string { return "unauthorized" }
func (e *ConflictError) Error() string    { return e.Message }

func NotFound(resource string) error     { return &NotFoundError{Resource: resource} }
func Validation(msg string) error        { return &ValidationError{Message: msg} }
func Unauthorized() error                { return &UnauthorizedError{} }
func Conflict(msg string) error          { return &ConflictError{Message: msg} }

func IsNotFound(err error) bool {
	_, ok := err.(*NotFoundError)
	return ok
}

func IsValidation(err error) bool {
	_, ok := err.(*ValidationError)
	return ok
}

func IsUnauthorized(err error) bool {
	_, ok := err.(*UnauthorizedError)
	return ok
}
