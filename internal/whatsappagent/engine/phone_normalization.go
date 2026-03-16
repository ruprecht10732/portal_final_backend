package engine

import "strings"

func normalizeAgentPhoneKey(value string) string {
	trimmed := strings.TrimSpace(strings.ToLower(value))
	if trimmed == "" {
		return ""
	}
	if at := strings.Index(trimmed, "@"); at >= 0 {
		trimmed = trimmed[:at]
	}
	trimmed = strings.ReplaceAll(trimmed, " ", "")
	trimmed = strings.ReplaceAll(trimmed, "-", "")
	trimmed = strings.ReplaceAll(trimmed, "(", "")
	trimmed = strings.ReplaceAll(trimmed, ")", "")
	if strings.HasPrefix(trimmed, "+") {
		return trimmed
	}
	return "+" + trimmed
}

func agentPhoneCandidates(value string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}

	seen := map[string]struct{}{}
	result := make([]string, 0, 4)
	add := func(candidate string) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			return
		}
		if _, ok := seen[candidate]; ok {
			return
		}
		seen[candidate] = struct{}{}
		result = append(result, candidate)
	}

	add(trimmed)
	normalized := normalizeAgentPhoneKey(trimmed)
	add(normalized)
	add(strings.TrimPrefix(normalized, "+"))
	if local, _, ok := strings.Cut(trimmed, "@"); ok {
		add(local)
		add(normalizeAgentPhoneKey(local))
	}

	return result
}