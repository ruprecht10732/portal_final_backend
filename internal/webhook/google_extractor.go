package webhook

import (
	"strconv"
	"strings"
)

// ExtractGoogleLeadFields maps Google Lead Form data into a flat map for generic extraction.
func ExtractGoogleLeadFields(payload GoogleLeadPayload) map[string]string {
	fields := make(map[string]string)

	for _, col := range payload.UserColumnData {
		key := normalizeGoogleFieldName(col.ColumnName)
		if key != "" && col.StringValue != "" {
			fields[key] = col.StringValue
		}
	}

	if payload.GCLID != "" {
		fields["gclid"] = payload.GCLID
	}
	if payload.GCLIDURL != "" {
		fields["landing_page"] = payload.GCLIDURL
	}

	return fields
}

// normalizeGoogleFieldName maps Google form field names to our internal field keys.
func normalizeGoogleFieldName(columnName string) string {
	label := strings.ToLower(strings.TrimSpace(columnName))

	switch {
	case containsAny(label, "first", "voornaam", "given"):
		return "firstName"
	case containsAny(label, "last", "achternaam", "surname", "family"):
		return "lastName"
	case containsAny(label, "full name", "name", "naam") && !containsAny(label, "first", "last"):
		return "fullName"
	case containsAny(label, "email", "e-mail"):
		return "email"
	case containsAny(label, "phone", "telefoon", "tel", "mobile", "mobiel"):
		return "phone"
	case containsAny(label, "postcode", "postal", "zip"):
		return "zipCode"
	case containsAny(label, "city", "stad", "plaats", "woonplaats"):
		return "city"
	case containsAny(label, "street", "straat", "address", "adres"):
		return "street"
	case containsAny(label, "house", "huisnummer", "number", "nr"):
		return "houseNumber"
	case containsAny(label, "message", "bericht", "opmerking", "comment", "vraag"):
		return "message"
	case containsAny(label, "company", "bedrijf"):
		return "companyName"
	default:
		return strings.TrimSpace(columnName)
	}
}

func containsAny(haystack string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(haystack, needle) {
			return true
		}
	}
	return false
}

// FindServiceTypeForCampaign resolves the service type for a campaign ID.
func FindServiceTypeForCampaign(config GoogleWebhookConfig, campaignID int64) string {
	key := strconv.FormatInt(campaignID, 10)
	if config.CampaignMappings == nil {
		return ""
	}
	return config.CampaignMappings[key]
}
