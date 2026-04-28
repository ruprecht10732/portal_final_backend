package agent

import (
	"fmt"
	"log"
	"sort"
	"strings"
)

// ──────────────────────────────────────────────────
// String canonicalisation helpers
// ──────────────────────────────────────────────────

// normalizeUrgencyLevel converts various urgency level formats to the required values: High, Medium, Low.
func normalizeUrgencyLevel(level string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(level))

	switch normalized {
	case "high", "hoog", "urgent", "spoed", "spoedeisend", "critical":
		return "High", nil
	case "medium", "mid", "moderate", "matig", "gemiddeld", "normal", "standard", "standaard":
		return "Medium", nil
	case "low", "laag", "non-urgent", "niet-urgent", "minor":
		return "Low", nil
	default:
		log.Printf("Unrecognized urgency level %q, defaulting to Medium", level)
		return "Medium", nil
	}
}

// normalizeLeadQuality converts various lead quality formats to the required values: Junk, Low, Potential, High, Urgent.
func normalizeLeadQuality(quality string) string {
	normalized := strings.ToLower(strings.TrimSpace(quality))

	switch normalized {
	case "junk", "spam", "rommel", "onzin", "fake":
		return "Junk"
	case "low", "laag":
		return "Low"
	case "potential", "potentieel", "medium", "gemiddeld", "moderate", "mid",
		"onbekend", "unknown", "niet bekend", "nvt", "n/a", "n.v.t.", "onbepaald":
		return "Potential"
	case "high", "hoog", "good", "goed", "qualified", "gekwalificeerd":
		return "High"
	case "urgent", "spoed", "critical", "kritiek":
		return "Urgent"
	default:
		log.Printf("Unrecognized lead quality %q, defaulting to Potential", quality)
		return "Potential"
	}
}

// normalizeRecommendedAction converts various action formats to valid values: Reject, RequestInfo, ScheduleSurvey, CallImmediately.
func normalizeRecommendedAction(action string) string {
	normalized := strings.ToLower(strings.TrimSpace(action))

	switch normalized {
	case "reject", "afwijzen", "weigeren":
		return "Reject"
	case "requestinfo", "request_info", "request info":
		return "RequestInfo"
	case "movetoestimation", "move_to_estimation", "move to estimation",
		"proceedtoestimation", "proceed_to_estimation", "proceed to estimation",
		"estimate", "estimateready", "estimate_ready", "estimate ready":
		return "ScheduleSurvey"
	case "schedulesurvey", "schedule_survey", "schedule survey", "survey", "opname", "inmeten":
		return "ScheduleSurvey"
	case "callimmediately", "call_immediately", "call immediately", "call", "bellen":
		return "CallImmediately"
	}

	if strings.Contains(normalized, "reject") || strings.Contains(normalized, "spam") || strings.Contains(normalized, "junk") {
		return "Reject"
	}
	if strings.Contains(normalized, "call") || strings.Contains(normalized, "bel") || strings.Contains(normalized, "phone") {
		return "CallImmediately"
	}
	if strings.Contains(normalized, "estimat") || strings.Contains(normalized, "proceed") || strings.Contains(normalized, "move to estimation") {
		return "ScheduleSurvey"
	}
	if strings.Contains(normalized, "survey") || strings.Contains(normalized, "opname") || strings.Contains(normalized, "inmeten") || strings.Contains(normalized, "schedule") {
		return "ScheduleSurvey"
	}
	if strings.Contains(normalized, "info") || strings.Contains(normalized, "contact") ||
		strings.Contains(normalized, "nurtur") || strings.Contains(normalized, "clarif") ||
		strings.Contains(normalized, "request") || strings.Contains(normalized, "more") ||
		strings.Contains(normalized, "review") {
		return "RequestInfo"
	}

	log.Printf("Unrecognized recommended action %q, defaulting to RequestInfo", action)
	return "RequestInfo"
}

// normalizeConsumerRole canonicalises consumer role strings.
func normalizeConsumerRole(role string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(role))
	switch normalized {
	case "owner":
		return "Owner", nil
	case "tenant":
		return "Tenant", nil
	case "landlord":
		return "Landlord", nil
	default:
		return "", fmt.Errorf("invalid consumer role")
	}
}

// normalizeContactChannel maps free-form channel names to canonical WhatsApp or Email.
func normalizeContactChannel(channel string) (string, error) {
	clean := strings.TrimSpace(channel)
	normalized := strings.ToLower(clean)

	if strings.Contains(normalized, "whatsapp") || normalized == "wa" {
		return "WhatsApp", nil
	}
	if strings.Contains(normalized, "email") || strings.Contains(normalized, "e-mail") || normalized == "mail" {
		return "Email", nil
	}
	if strings.Contains(normalized, "phone") || strings.Contains(normalized, "telefoon") ||
		strings.Contains(normalized, "call") || strings.Contains(normalized, "bel") ||
		normalized == "tel" || normalized == "sms" {
		return "WhatsApp", nil
	}

	log.Printf("Unrecognized contact channel %q, defaulting to Email", channel)
	return "Email", nil
}

// normalizeMissingInformation trims, de-duplicates and sorts missing-info items.
func normalizeMissingInformation(items []string) []string {
	normalized := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		normalized = append(normalized, trimmed)
	}
	sort.Strings(normalized)
	return normalized
}

// normalizeExtractedFacts trims keys/values and filters out empty entries.
func normalizeExtractedFacts(facts map[string]string) map[string]string {
	if len(facts) == 0 {
		return map[string]string{}
	}
	normalized := make(map[string]string, len(facts))
	for key, value := range facts {
		trimmedKey := strings.TrimSpace(key)
		trimmedValue := strings.TrimSpace(value)
		if trimmedKey == "" || trimmedValue == "" {
			continue
		}
		normalized[trimmedKey] = trimmedValue
	}
	if len(normalized) == 0 {
		return map[string]string{}
	}
	return normalized
}

// contactMessageReplacer is compiled once and reused for all contact message normalisations.
var contactMessageReplacer = strings.NewReplacer(
	"\r\n", "\n",
	"\\n", "\n",
	"\\t", " ",
	"\r", "",
	"nodie", "nodig",
	"nodien", "nodig",
)

// normalizeSuggestedContactMessage cleans up AI-generated contact messages.
func normalizeSuggestedContactMessage(message string) string {
	if strings.TrimSpace(message) == "" {
		return ""
	}

	normalized := contactMessageReplacer.Replace(message)

	lines := strings.Split(normalized, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			lines[i] = ""
			continue
		}
		lines[i] = strings.Join(strings.Fields(line), " ")
	}
	normalized = strings.Join(lines, "\n")
	return strings.TrimSpace(normalized)
}

// normalizeGatekeeperLoopItems deduplicates and lower-cases loop-item strings.
func normalizeGatekeeperLoopItems(values []string) []string {
	set := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.ToLower(strings.TrimSpace(value))
		if trimmed == "" {
			continue
		}
		if _, exists := set[trimmed]; exists {
			continue
		}
		set[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	sort.Strings(normalized)
	return normalized
}

// ──────────────────────────────────────────────────
// Photo-analysis normalisation
// ──────────────────────────────────────────────────

func normalizeConfidenceLevel(level string) string {
	switch level {
	case "High", "HIGH", "high", "Hoog", "hoog":
		return "High"
	case "Low", "LOW", "low", "Laag", "laag":
		return "Low"
	default:
		return "Medium"
	}
}

func normalizeScopeAssessment(scope string) string {
	switch scope {
	case "Small", "SMALL", "small", "Klein", "klein", "Minor", "minor":
		return "Small"
	case "Medium", "MEDIUM", "medium", "Gemiddeld", "gemiddeld", "Moderate", "moderate":
		return "Medium"
	case "Large", "LARGE", "large", "Groot", "groot", "Major", "major", "Extensive", "extensive":
		return "Large"
	case "Unclear", "UNCLEAR", "unclear", "Onduidelijk", "onduidelijk", "Unknown", "unknown":
		return "Unclear"
	default:
		return "Unclear"
	}
}

func normalizeMeasurementType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "dimension", "length", "width", "height", "depth", "lengte", "breedte", "hoogte":
		return "dimension"
	case "area", "oppervlakte", "m2":
		return "area"
	case "count", "aantal", "stuks", "quantity":
		return "count"
	case "volume", "inhoud", "m3":
		return "volume"
	default:
		return "dimension"
	}
}

// ──────────────────────────────────────────────────
// Call-logger normalisation
// ──────────────────────────────────────────────────

// normalizeCallNoteBody strips metadata lines and collapses duplicate blank lines.
func normalizeCallNoteBody(body string) string {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return ""
	}

	lines := strings.Split(trimmed, "\n")
	cleaned := make([]string, 0, len(lines))
	lastBlank := false
	for _, line := range lines {
		plain := strings.TrimSpace(line)
		lower := strings.ToLower(plain)
		if strings.Contains(lower, "originele input") {
			continue
		}
		if plain == "" {
			if lastBlank {
				continue
			}
			lastBlank = true
			cleaned = append(cleaned, "")
			continue
		}
		lastBlank = false
		cleaned = append(cleaned, strings.TrimRight(line, " \t"))
	}

	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}

// ──────────────────────────────────────────────────
// Photo-preprocessor normalisation
// ──────────────────────────────────────────────────



// ──────────────────────────────────────────────────
// Council-service normalisation
// ──────────────────────────────────────────────────

func normalizeCouncilConsensusMode(mode string) string {
	normalized := strings.ToLower(strings.TrimSpace(mode))
	switch normalized {
	case "weighted", "majority", "estimator_final":
		return normalized
	default:
		return "weighted"
	}
}

func normalizeCouncilEvaluation(in CouncilEvaluation) CouncilEvaluation {
	if strings.TrimSpace(in.Decision) == "" {
		in.Decision = CouncilDecisionAllow
	}
	if strings.TrimSpace(in.ReasonCode) == "" {
		in.ReasonCode = "council_allow"
	}
	if strings.TrimSpace(in.Summary) == "" {
		in.Summary = "Council akkoord."
	}
	return in
}
