package validator

import (
	"unicode"

	"portal_final_backend/platform/validator"

	gpvalidator "github.com/go-playground/validator/v10"
)

// RegisterAuthValidations registers auth-specific validation rules on a validator instance.
// This should be called once during module initialization with the injected validator.
func RegisterAuthValidations(v *validator.Validator) error {
	return v.RegisterValidation("strongpassword", validateStrongPassword)
}

// validateStrongPassword checks for password complexity in $O(K)$ time where K is
// the number of characters evaluated before conditions are met.
// - At least 8 characters
// - At least one uppercase letter
// - At least one lowercase letter
// - At least one digit
// - At least one special character
func validateStrongPassword(fl gpvalidator.FieldLevel) bool {
	password := fl.Field().String()

	// Length constraints.
	// $O(1)$ fast-path rejections.
	l := len(password)
	if l < 8 || l > 1024 {
		return false
	}

	var hasUpper, hasLower, hasDigit, hasSpecial bool

	for _, char := range password {
		// Optimize by skipping Unicode checks once a flag is already true
		switch {
		case !hasUpper && unicode.IsUpper(char):
			hasUpper = true
		case !hasLower && unicode.IsLower(char):
			hasLower = true
		case !hasDigit && unicode.IsDigit(char):
			hasDigit = true
		case !hasSpecial && (unicode.IsPunct(char) || unicode.IsSymbol(char)):
			hasSpecial = true
		}

		// Optimization: Short-circuit loop early.
		if hasUpper && hasLower && hasDigit && hasSpecial {
			return true
		}
	}

	return false
}
