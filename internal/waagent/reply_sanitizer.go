package waagent

import (
	"regexp"
	"strings"
)

var (
	reHeadingLine       = regexp.MustCompile(`(?m)^#{1,6}\s*(.+?)\s*$`)
	reHorizontalRule    = regexp.MustCompile(`(?m)^\s*---+\s*$`)
	reUUID              = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}\b`)
	reLeadIDClause      = regexp.MustCompile(`(?i)\(?\b(?:lead|service|quote|offerte|appointment)\s*id\s*:\s*[A-Za-z0-9_-]+\)?`)
	reMetaPreamble      = regexp.MustCompile(`(?im)^(ik ga [^.\n]*?(opzoeken|zoeken|maken|opslaan|inplannen|controleren)\.?\s*)`)
	reLetMePreamble     = regexp.MustCompile(`(?im)^(laat me [^.\n]*?\.?\s*)`)
	reDoubleStar        = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reEmptyBullet       = regexp.MustCompile(`(?m)^\s*[-*]\s*$`)
	reMultipleBlankLine = regexp.MustCompile(`\n{3,}`)
)

func sanitizeWhatsAppReply(reply string) string {
	text := strings.ReplaceAll(reply, "\r\n", "\n")
	text = convertMarkdownTables(text)
	text = reHeadingLine.ReplaceAllString(text, `*$1*`)
	text = reHorizontalRule.ReplaceAllString(text, "")
	text = reDoubleStar.ReplaceAllString(text, `*$1*`)
	text = reLeadIDClause.ReplaceAllString(text, "")
	text = reUUID.ReplaceAllString(text, "")
	text = reMetaPreamble.ReplaceAllString(text, "")
	text = reLetMePreamble.ReplaceAllString(text, "")
	text = reEmptyBullet.ReplaceAllString(text, "")
	text = reMultipleBlankLine.ReplaceAllString(text, "\n\n")
	lines := strings.Split(text, "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			cleaned = append(cleaned, "")
			continue
		}
		cleaned = append(cleaned, strings.TrimSpace(trimmed))
	}
	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}

func convertMarkdownTables(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); {
		if !looksLikeTableHeader(lines, i) {
			out = append(out, lines[i])
			i++
			continue
		}
		headers := parseMarkdownTableRow(lines[i])
		i += 2
		for i < len(lines) && looksLikeTableRow(lines[i]) {
			cells := parseMarkdownTableRow(lines[i])
			out = append(out, formatTableRow(headers, cells))
			i++
		}
	}
	return strings.Join(out, "\n")
}

func looksLikeTableHeader(lines []string, idx int) bool {
	if idx+1 >= len(lines) {
		return false
	}
	return looksLikeTableRow(lines[idx]) && looksLikeTableSeparator(lines[idx+1])
}

func looksLikeTableRow(line string) bool {
	return strings.Count(line, "|") >= 2
}

func looksLikeTableSeparator(line string) bool {
	trimmed := strings.TrimSpace(line)
	if !looksLikeTableRow(trimmed) {
		return false
	}
	trimmed = strings.ReplaceAll(trimmed, "|", "")
	trimmed = strings.ReplaceAll(trimmed, ":", "")
	trimmed = strings.ReplaceAll(trimmed, "-", "")
	return strings.TrimSpace(trimmed) == ""
}

func parseMarkdownTableRow(line string) []string {
	parts := strings.Split(line, "|")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		items = append(items, trimmed)
	}
	return items
}

func formatTableRow(headers, cells []string) string {
	parts := make([]string, 0, len(cells))
	for i, cell := range cells {
		if i < len(headers) && headers[i] != "" {
			parts = append(parts, headers[i]+": "+cell)
			continue
		}
		parts = append(parts, cell)
	}
	if len(parts) == 0 {
		return ""
	}
	return "- " + strings.Join(parts, ", ")
}
