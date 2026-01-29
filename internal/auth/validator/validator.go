package validator

import (
	"regexp"
	"unicode"

	"github.com/go-playground/validator/v10"
)

var Validate = validator.New()

func init() {
	// Register custom password validation
	_ = Validate.RegisterValidation("strongpassword", validateStrongPassword)
}

// validateStrongPassword checks for password complexity:
// - At least 8 characters
// - At least one uppercase letter
// - At least one lowercase letter
// - At least one digit
// - At least one special character
func validateStrongPassword(fl validator.FieldLevel) bool {
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

// IsValidEmail validates email format
func IsValidEmail(email string) bool {
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	return emailRegex.MatchString(email)
}
