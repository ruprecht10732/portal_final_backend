package agent

import (
	"fmt"
	"strings"
	"unicode"
)

const (
	maxNoteLength    = 2000
	maxConsumerNote  = 1000
	userDataBegin    = "<user_input>"
	userDataEnd      = "</user_input>"
	dateTimeLayout   = "02-01-2006 15:04"
	dateLayout       = "02-01-2006"
	bulletLine       = "- %s\n"
	valueNotProvided = "Niet opgegeven"
)

// sanitizeUserInput removes control characters and truncates to max length.
func sanitizeUserInput(s string, maxLen int) string {
	var sb strings.Builder
	for _, r := range s {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			continue
		}
		sb.WriteRune(r)
	}
	result := sb.String()
	if len(result) > maxLen {
		result = result[:maxLen] + "... [afgekapt]"
	}
	return result
}

// escapeXMLText escapes user-provided text so it cannot break out of the XML-ish wrapper.
// This is not full XML serialization; it's a pragmatic guard against prompt injection.
func escapeXMLText(s string) string {
	// Order matters: escape '&' first.
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&apos;",
	)
	return r.Replace(s)
}

// wrapUserData wraps user-provided content with XML-style tags to isolate it from instructions.
func wrapUserData(content string) string {
	// Escape so the user cannot inject closing tags like </user_input>.
	return fmt.Sprintf("%s\n%s\n%s", userDataBegin, escapeXMLText(content), userDataEnd)
}

func getValue(s *string) string {
	if s == nil {
		return valueNotProvided
	}
	return *s
}
