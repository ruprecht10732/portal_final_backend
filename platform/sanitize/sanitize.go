// Package sanitize provides text sanitization utilities to prevent XSS attacks.
package sanitize

import (
	"regexp"
	"strings"
)

var (
	// htmlTagRegex matches HTML tags
	htmlTagRegex = regexp.MustCompile(`<[^>]*>`)
	// scriptRegex matches script-like patterns that might bypass simple tag stripping
	scriptRegex = regexp.MustCompile(`(?i)(javascript|on\w+\s*=|<script|<\/script)`)
)

// StripHTML removes all HTML tags from a string, making it safe for text-only display.
// This is a defense-in-depth measure; frontend should also escape output.
func StripHTML(s string) string {
	// Remove HTML tags
	result := htmlTagRegex.ReplaceAllString(s, "")
	// Decode common HTML entities
	result = strings.ReplaceAll(result, "&lt;", "<")
	result = strings.ReplaceAll(result, "&gt;", ">")
	result = strings.ReplaceAll(result, "&amp;", "&")
	result = strings.ReplaceAll(result, "&quot;", "\"")
	result = strings.ReplaceAll(result, "&#39;", "'")
	// Re-strip after entity decode to catch encoded tags
	result = htmlTagRegex.ReplaceAllString(result, "")
	return strings.TrimSpace(result)
}

// StripHTMLPtr is a helper for optional string pointers
func StripHTMLPtr(s *string) *string {
	if s == nil {
		return nil
	}
	result := StripHTML(*s)
	return &result
}

// Text sanitizes a string for safe text storage by stripping HTML
// and normalizing whitespace. Use for user-provided text fields like
// descriptions, notes, and comments.
func Text(s string) string {
	return StripHTML(s)
}

// TextPtr is a helper for optional string pointers
func TextPtr(s *string) *string {
	if s == nil {
		return nil
	}
	result := Text(*s)
	return &result
}

// ContainsDangerousPatterns checks if a string contains potentially dangerous patterns.
// Returns true if the string might contain XSS attempts.
func ContainsDangerousPatterns(s string) bool {
	return scriptRegex.MatchString(s)
}
