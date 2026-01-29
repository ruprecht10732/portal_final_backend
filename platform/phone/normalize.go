// Package phone provides phone number utilities.
// This is part of the platform layer and contains no business logic.
package phone

import (
	"strings"

	"github.com/nyaruka/phonenumbers"
)

const defaultRegion = "NL"

// NormalizeE164 formats a phone number to E.164. If parsing fails, it returns the trimmed input.
func NormalizeE164(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return trimmed
	}

	number, err := phonenumbers.Parse(trimmed, defaultRegion)
	if err != nil {
		return trimmed
	}

	if !phonenumbers.IsValidNumber(number) {
		return trimmed
	}

	return phonenumbers.Format(number, phonenumbers.E164)
}
