package webhook

import (
	"regexp"
	"strings"
)

// ExtractedFields holds the fields extracted from raw form data via best-effort pattern matching.
type ExtractedFields struct {
	FirstName   string
	LastName    string
	Email       string
	Phone       string
	Street      string
	HouseNumber string
	ZipCode     string
	City        string
	Message     string
	ServiceType string // Matched against known service type slugs/keywords
}

// IsIncomplete returns true if minimum required fields (name + at least one contact method) are missing.
func (e ExtractedFields) IsIncomplete() bool {
	hasName := e.FirstName != "" || e.LastName != ""
	hasContact := e.Phone != "" || e.Email != ""
	return !hasName || !hasContact
}

// ExtractFields performs best-effort field extraction from a flat string map of form data.
// It uses label matching to identify common fields across any form.
func ExtractFields(data map[string]string) ExtractedFields {
	var result ExtractedFields

	for key, value := range data {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(key))

		switch {
		case matchesAny(k, firstNamePatterns):
			result.FirstName = value
		case matchesAny(k, lastNamePatterns):
			result.LastName = value
		case matchesAny(k, fullNamePatterns):
			parts := strings.SplitN(value, " ", 2)
			result.FirstName = parts[0]
			if len(parts) > 1 {
				result.LastName = parts[1]
			}
		case matchesAny(k, emailPatterns):
			if emailRegex.MatchString(value) {
				result.Email = value
			}
		case matchesAny(k, phonePatterns):
			result.Phone = normalizePhone(value)
		case matchesAny(k, streetPatterns):
			result.Street = value
		case matchesAny(k, houseNumberPatterns):
			result.HouseNumber = value
		case matchesAny(k, zipCodePatterns):
			result.ZipCode = normalizeZipCode(value)
		case matchesAny(k, cityPatterns):
			result.City = value
		case matchesAny(k, messagePatterns):
			result.Message = value
		case matchesAny(k, addressPatterns):
			// Full address: try to parse Dutch-style "Street HouseNumber" or "Street HouseNumber, ZipCode City"
			parseFullAddress(value, &result)
		case matchesAny(k, serviceTypePatterns):
			// Try to map the value to a known service type slug
			if st := matchServiceType(value); st != "" {
				result.ServiceType = st
			}
		}
	}

	// If we got a full name but no separate first/last, and first name looks like "first last"
	if result.FirstName != "" && result.LastName == "" && strings.Contains(result.FirstName, " ") {
		parts := strings.SplitN(result.FirstName, " ", 2)
		result.FirstName = parts[0]
		result.LastName = parts[1]
	}

	return result
}

// Field label patterns (Dutch + English)
var (
	firstNamePatterns   = []string{"first_name", "firstname", "first name", "voornaam", "given_name", "givenname", "fname"}
	lastNamePatterns    = []string{"last_name", "lastname", "last name", "achternaam", "family_name", "familyname", "surname", "lname"}
	fullNamePatterns    = []string{"name", "naam", "full_name", "fullname", "your_name", "your name"}
	emailPatterns       = []string{"email", "e-mail", "e_mail", "emailaddress", "email_address", "mail"}
	phonePatterns       = []string{"phone", "telefoon", "tel", "telephone", "phonenumber", "phone_number", "telefoonnummer", "mobile", "mobiel", "gsm"}
	streetPatterns      = []string{"street", "straat", "straatnaam", "street_name", "streetname"}
	houseNumberPatterns = []string{"house_number", "housenumber", "huisnummer", "house number", "nr", "number", "nummer"}
	zipCodePatterns     = []string{"zip", "zipcode", "zip_code", "postcode", "postal_code", "postalcode", "zip code", "postal code"}
	cityPatterns        = []string{"city", "stad", "woonplaats", "plaats", "town", "gemeente", "location", "locatie"}
	messagePatterns     = []string{"message", "bericht", "opmerking", "opmerkingen", "comment", "comments", "notes", "description", "toelichting", "vraag", "question"}
	addressPatterns     = []string{"address", "adres", "full_address", "fulladdress"}
	serviceTypePatterns = []string{"service", "dienst", "project_type", "projecttype", "service_type", "servicetype", "type", "werkzaamheden", "soort", "category", "categorie", "product"}
)

var (
	emailRegex   = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	dutchZipRe   = regexp.MustCompile(`(\d{4})\s*([A-Za-z]{2})`)
	dutchPhoneRe = regexp.MustCompile(`^(?:\+31|0031|0)\s*[-.\s]?(\d[\d\s\-\.]{7,})$`)
)

func matchesAny(label string, patterns []string) bool {
	// Normalize: strip spaces, dashes, underscores for fuzzy matching
	normalized := strings.NewReplacer("-", "", "_", "", " ", "").Replace(label)
	for _, p := range patterns {
		pNormalized := strings.NewReplacer("-", "", "_", "", " ", "").Replace(p)
		if normalized == pNormalized {
			return true
		}
	}
	return false
}

func normalizePhone(value string) string {
	// Remove common formatting characters
	cleaned := strings.Map(func(r rune) rune {
		if (r >= '0' && r <= '9') || r == '+' {
			return r
		}
		return -1
	}, value)

	// Dutch number normalization: 06... → +316...
	if strings.HasPrefix(cleaned, "06") && len(cleaned) == 10 {
		return "+31" + cleaned[1:]
	}
	if strings.HasPrefix(cleaned, "0031") {
		return "+" + cleaned[2:]
	}
	if strings.HasPrefix(cleaned, "0") && len(cleaned) == 10 {
		return "+31" + cleaned[1:]
	}

	return cleaned
}

func normalizeZipCode(value string) string {
	value = strings.TrimSpace(value)
	m := dutchZipRe.FindStringSubmatch(value)
	if len(m) == 3 {
		return m[1] + " " + strings.ToUpper(m[2])
	}
	return value
}

func parseFullAddress(value string, result *ExtractedFields) {
	// Try "Street HouseNumber, ZipCode City" or "Street HouseNumber"
	parts := strings.SplitN(value, ",", 2)
	streetPart := strings.TrimSpace(parts[0])

	// Extract house number from street part (last word if it starts with a digit)
	words := strings.Fields(streetPart)
	if len(words) >= 2 {
		last := words[len(words)-1]
		if len(last) > 0 && last[0] >= '0' && last[0] <= '9' {
			if result.Street == "" {
				result.Street = strings.Join(words[:len(words)-1], " ")
			}
			if result.HouseNumber == "" {
				result.HouseNumber = last
			}
		} else if result.Street == "" {
			result.Street = streetPart
		}
	}

	if len(parts) == 2 {
		rest := strings.TrimSpace(parts[1])
		// Try to extract zip code and city
		m := dutchZipRe.FindStringSubmatchIndex(rest)
		if m != nil {
			if result.ZipCode == "" {
				result.ZipCode = normalizeZipCode(rest[m[0]:m[1]])
			}
			after := strings.TrimSpace(rest[m[1]:])
			if result.City == "" && after != "" {
				result.City = after
			}
		} else if result.City == "" {
			result.City = rest
		}
	}
}

// serviceTypeKeywords maps Dutch + English keywords found in form values
// to the slugs used in RAC_service_types. Order matters: first match wins.
var serviceTypeKeywords = []struct {
	slug     string
	keywords []string
}{
	{"windows", []string{"kozijn", "kozijnen", "raam", "ramen", "window", "windows", "glas", "glaswerk", "deur", "deuren", "door", "doors", "beglazing", "dubbel glas", "hr++", "triple glas"}},
	{"insulation", []string{"isolatie", "isoleren", "insulation", "spouwmuur", "vloerisolatie", "dakisolatie", "muurisolatie", "cavity wall", "floor insulation", "roof insulation"}},
	{"solar", []string{"solar", "zonnepaneel", "zonnepanelen", "zonne-energie", "pv", "zonneboiler", "solar panel"}},
	{"hvac", []string{"hvac", "warmtepomp", "heat pump", "airco", "airconditioning", "verwarming", "heating", "ventilatie", "ventilation", "cv", "cv-ketel", "boiler"}},
	{"plumbing", []string{"loodgieter", "plumbing", "sanitair", "afvoer", "drain", "waterleiding", "badkamer", "bathroom", "kraan", "toilet"}},
	{"electrical", []string{"elektra", "electrical", "electrician", "bedrading", "wiring", "meterkast", "laadpaal", "charging station", "ev charger"}},
	{"carpentry", []string{"timmerwerk", "timmerman", "carpentry", "carpenter", "hout", "wood", "vloer", "floor", "parket", "laminaat", "trap", "stairs"}},
	{"handyman", []string{"klusjesman", "handyman", "klussen", "reparatie", "repair", "onderhoud", "maintenance"}},
}

// matchServiceType maps a form field value to a known service type slug
// using keyword matching. Returns empty string if no match.
func matchServiceType(value string) string {
	v := strings.ToLower(strings.TrimSpace(value))
	if v == "" {
		return ""
	}
	for _, st := range serviceTypeKeywords {
		for _, kw := range st.keywords {
			if strings.Contains(v, kw) {
				return st.slug
			}
		}
	}
	return ""
}
