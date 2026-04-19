package validator

import (
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

func Validate(v validation.Validatable) error {
	return v.Validate()
}
