package validator

import (
	"unicode"

	"portal_final_backend/platform/validator"

	gpvalidator "github.com/go-playground/validator/v10"
)

// Validate is an alias to the platform validator for convenience within the auth domain.
// DEPRECATED: Use injected validator instead. This is kept for backward compatibility.
var Validate = validator.Validate

// RegisterAuthValidations registers auth-specific validation rules on a validator instance.
// This should be called once during module initialization with the injected validator.
func RegisterAuthValidations(v *validator.Validator) error {
	return v.RegisterValidation("strongpassword", validateStrongPassword)
}

func init() {
	// Register on the global validator for backward compatibility
	_ = Validate.RegisterValidation("strongpassword", validateStrongPassword)
}

// validateStrongPassword checks for password complexity:
// - At least 8 characters
// - At least one uppercase letter
// - At least one lowercase letter
// - At least one digit
// - At least one special character
func validateStrongPassword(fl gpvalidator.FieldLevel) bool {
	password := fl.Field().String()

	if len(password) < 8 {
		return false
	}

	var (
		hasUpper   bool
		hasLower   bool
		hasDigit   bool
		hasSpecial bool
	)

	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsDigit(char):
			hasDigit = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}

	return hasUpper && hasLower && hasDigit && hasSpecial
}

// PasswordPolicy describes the password requirements for API error messages
const PasswordPolicy = "Password must be at least 8 characters and include: uppercase letter, lowercase letter, number, and special character"
