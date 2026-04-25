// Package validator provides a thin wrapper around ozzo-validation.
package validator

import (
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Validatable is implemented by any struct that carries its own validation rules.
type Validatable interface {
	Validate() error
}

// Validate calls ozzo-validation on any struct that implements validation.Validatable.
// Pass an ozzo-validation.Validatable value, or use the ozzo package directly.
var (
	// Required re-exports the ozzo Required rule for convenience.
	Required = validation.Required
	// Validate re-exports ozzo ValidateStruct for convenience.
	ValidateStruct = validation.ValidateStruct
	// Field re-exports ozzo Field for struct field validation.
	Field = validation.Field
)
