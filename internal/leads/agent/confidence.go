package agent

import (
	"strings"

	"portal_final_backend/internal/leads/repository"
)

type confidenceResult struct {
	Score     float64
	Breakdown map[string]float64
	RiskFlags []string
}

func calculateAnalysisConfidence(lead repository.Lead, leadQuality, recommendedAction string, missingInformation []string, photoAnalysis *repository.PhotoAnalysis) confidenceResult {
	llmCertainty := confidenceFromLeadQuality(leadQuality)
	dataCompleteness, completenessFlags := calculateLeadDataCompleteness(lead)
	extractionReliability, extractionFlags := confidenceFromPhotoAnalysis(photoAnalysis)
	businessValidation, validationFlags := calculateBusinessValidation(lead, recommendedAction, missingInformation)

	score := clamp01(
		0.35*llmCertainty +
			0.25*dataCompleteness +
			0.20*extractionReliability +
			0.20*businessValidation,
	)

	riskFlags := make([]string, 0, 8)
	riskFlags = append(riskFlags, completenessFlags...)
	riskFlags = append(riskFlags, extractionFlags...)
	riskFlags = append(riskFlags, validationFlags...)

	if llmCertainty < 0.45 {
		riskFlags = append(riskFlags, "low_llm_certainty")
	}

	return confidenceResult{
		Score: score,
		Breakdown: map[string]float64{
			"llmCertainty":          llmCertainty,
			"dataCompleteness":      dataCompleteness,
			"extractionReliability": extractionReliability,
			"businessValidation":    businessValidation,
		},
		RiskFlags: dedupeStrings(riskFlags),
	}
}

func confidenceFromPhotoAnalysis(photoAnalysis *repository.PhotoAnalysis) (float64, []string) {
	if photoAnalysis == nil {
		return 0.50, []string{"no_photo_analysis"}
	}

	confidence := strings.ToLower(strings.TrimSpace(photoAnalysis.ConfidenceLevel))
	switch confidence {
	case "high":
		return 0.90, nil
	case "medium":
		return 0.65, nil
	case "low":
		return 0.30, []string{"low_photo_confidence"}
	default:
		if len(photoAnalysis.Discrepancies) > 0 {
			return 0.45, []string{"photo_discrepancies_present"}
		}
		return 0.50, []string{"unknown_photo_confidence"}
	}
}

func confidenceFromLeadQuality(leadQuality string) float64 {
	switch strings.ToLower(strings.TrimSpace(leadQuality)) {
	case "junk":
		return 0.10
	case "low":
		return 0.30
	case "potential":
		return 0.60
	case "high":
		return 0.80
	case "urgent":
		return 0.90
	default:
		return 0.50
	}
}

func calculateLeadDataCompleteness(lead repository.Lead) (float64, []string) {
	filled := 0.0
	total := 7.0
	flags := make([]string, 0, 4)

	if strings.TrimSpace(lead.ConsumerFirstName) != "" {
		filled++
	} else {
		flags = append(flags, "missing_first_name")
	}
	if strings.TrimSpace(lead.ConsumerLastName) != "" {
		filled++
	} else {
		flags = append(flags, "missing_last_name")
	}
	if strings.TrimSpace(lead.ConsumerPhone) != "" {
		filled++
	} else {
		flags = append(flags, "missing_phone")
	}
	if strings.TrimSpace(lead.AddressStreet) != "" {
		filled++
	} else {
		flags = append(flags, "missing_street")
	}
	if strings.TrimSpace(lead.AddressHouseNumber) != "" {
		filled++
	} else {
		flags = append(flags, "missing_house_number")
	}
	if strings.TrimSpace(lead.AddressZipCode) != "" {
		filled++
	} else {
		flags = append(flags, "missing_zip_code")
	}
	if strings.TrimSpace(lead.AddressCity) != "" {
		filled++
	} else {
		flags = append(flags, "missing_city")
	}

	return filled / total, flags
}

func calculateBusinessValidation(lead repository.Lead, recommendedAction string, missingInformation []string) (float64, []string) {
	score := 1.0
	flags := make([]string, 0, 4)

	if len(normalizeMissingInformation(missingInformation)) > 0 {
		score -= 0.30
		flags = append(flags, "missing_information")
	}

	if strings.EqualFold(strings.TrimSpace(recommendedAction), "RequestInfo") {
		score -= 0.20
		flags = append(flags, "request_info_action")
	}

	if strings.TrimSpace(lead.ConsumerPhone) == "" {
		score -= 0.20
		flags = append(flags, "no_contact_phone")
	}

	return clamp01(score), flags
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		k := strings.TrimSpace(value)
		if k == "" {
			continue
		}
		if _, exists := seen[k]; exists {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
