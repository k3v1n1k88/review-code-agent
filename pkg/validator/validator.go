package validator

import (
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// Validate runs ozzo-validation rules against a validatable value.
// Returns a map of field → error strings, or nil if valid.
func Validate(v validation.Validatable) error {
	return v.Validate()
}

// ValidateValue validates a single value against the given rules.
func ValidateValue(value interface{}, rules ...validation.Rule) error {
	return validation.Validate(value, rules...)
}
