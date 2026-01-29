// Package validator provides a shared validation instance for use across all
// bounded contexts. Domain-specific validation rules should be registered in
// their respective domains.
package validator

import "github.com/go-playground/validator/v10"

// Validate is the shared validator instance used across all modules.
// Domain-specific validation rules can be registered in their respective
// domain packages using RegisterValidation.
var Validate = validator.New()
