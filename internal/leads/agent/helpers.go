package agent

import (
"fmt"
"strings"
"unicode"

"portal_final_backend/internal/leads/repository"
)

const (
maxNoteLength    = 2000
maxConsumerNote  = 1000
userDataBegin    = "<<<BEGIN_USER_DATA>>>"
userDataEnd      = "<<<END_USER_DATA>>>"
dateTimeLayout   = "02-01-2006 15:04"
dateLayout       = "02-01-2006"
bulletLine       = "- %s\n"
valueNotProvided = "Niet opgegeven"
)

// filterMeaningfulNotes filters out system notes that don't count as meaningful information.
func filterMeaningfulNotes(notes []repository.LeadNote) []repository.LeadNote {
const noteTypeSystem = "system"
var meaningful []repository.LeadNote
for _, note := range notes {
if note.Type != noteTypeSystem {
meaningful = append(meaningful, note)
}
}
return meaningful
}

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

// wrapUserData wraps user-provided content with markers to isolate it from instructions.
func wrapUserData(content string) string {
return fmt.Sprintf("%s\n%s\n%s", userDataBegin, content, userDataEnd)
}

func getValue(s *string) string {
if s == nil {
return valueNotProvided
}
return *s
}
