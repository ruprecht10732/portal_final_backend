// Package validator provides validation infrastructure for the application.
// This is part of the platform layer and contains no business logic.
package validator

import "github.com/go-playground/validator/v10"

// Validator wraps the go-playground validator for structured validation.
// Using a struct allows for dependency injection and easier testing.
type Validator struct {
	v *validator.Validate
}

// New creates a new Validator instance.
// Domain-specific validation rules can be registered using RegisterValidation.
func New() *Validator {
	return &Validator{
		v: validator.New(),
	}
}

// Struct validates a struct based on validation tags.
func (val *Validator) Struct(s interface{}) error {
	return val.v.Struct(s)
}

// Var validates a single variable against a tag.
func (val *Validator) Var(field interface{}, tag string) error {
	return val.v.Var(field, tag)
}

// RegisterValidation registers a custom validation function.
func (val *Validator) RegisterValidation(tag string, fn validator.Func) error {
	return val.v.RegisterValidation(tag, fn)
}

// Validate is the shared validator instance used across all modules.
// DEPRECATED: Use New() to create and inject a Validator instance instead.
// This global is kept for backward compatibility during migration.
var Validate = validator.New()
