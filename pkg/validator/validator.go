package validator

import (
	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-ozzo/ozzo-validation/v4/is"
)

// Expose commonly used rules for convenience.
var (
	Required = validation.Required
	Email    = is.Email
	UUID     = is.UUID
)

// Struct validates a validatable struct (implements validation.Validatable).
func Struct(v validation.Validatable) error {
	return v.Validate()
}
