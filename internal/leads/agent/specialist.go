package agent

import (
	"fmt"
	"strings"
)

// suggestSpecialist recommends the right type of specialist based on problem description
func suggestSpecialist(problemDescription, categoryHint string) SuggestSpecialistOutput {
	problem := strings.ToLower(problemDescription)
	hint := strings.ToLower(strings.TrimSpace(categoryHint))

	// Emergency indicators
	isEmergency := isEmergencyProblem(problem)

	// Plumbing indicators
	plumbingKeywords := []string{"kraan", "lek", "toilet", "douche", "bad", "afvoer", "verstopt", "water", "boiler", "geiser", "cv-ketel", "leiding"}
	plumbingScore := countKeywords(problem, plumbingKeywords)

	// HVAC indicators
	hvacKeywords := []string{"verwarming", "cv", "ketel", "airco", "warmtepomp", "thermostaat", "radiator", "vloerverwarming", "koeling", "warm water"}
	hvacScore := countKeywords(problem, hvacKeywords)

	// Electrical indicators
	electricalKeywords := []string{"stroom", "elektr", "stopcontact", "schakelaar", "lamp", "verlichting", "meterkast", "groepenkast", "kortsluiting", "zekering", "laadpaal"}
	electricalScore := countKeywords(problem, electricalKeywords)

	// Carpentry indicators
	carpentryKeywords := []string{"deur", "raam", "kozijn", "vloer", "laminaat", "parket", "kast", "keuken", "plint", "trap", "hout", "meubel"}
	carpentryScore := countKeywords(problem, carpentryKeywords)

	applyCategoryHintBoost(hint, &plumbingScore, &hvacScore, &electricalScore, &carpentryScore)

	winner, alternatives := determineWinnerAndAlternatives(plumbingScore, hvacScore, electricalScore, carpentryScore)

	// If top score is 0, default to handyman
	if winner.score == 0 {
		return SuggestSpecialistOutput{
			RecommendedSpecialist: "Klusjesman (all-round)",
			Reason:                "Op basis van de beschrijving is het niet duidelijk welke specialist nodig is. Een ervaren klusjesman kan de situatie beoordelen.",
			AlternativeOptions:    []string{"Loodgieter", "Elektricien", "Timmerman"},
			QuestionsToAsk: []string{
				"Kunt u het probleem wat specifieker beschrijven?",
				"Gaat het om water, elektra, of iets met hout/deuren?",
				"Wanneer is het probleem ontstaan?",
			},
		}
	}

	questions := questionsForSpecialist(winner.name)

	reason := fmt.Sprintf("Op basis van de beschrijving ('%s') lijkt een %s het meest geschikt.",
		truncateString(problemDescription, 50), winner.name)
	if isEmergency {
		reason = "SPOED: " + reason + " Directe inzet aanbevolen."
	}

	return SuggestSpecialistOutput{
		RecommendedSpecialist: winner.name,
		Reason:                reason,
		AlternativeOptions:    alternatives,
		QuestionsToAsk:        questions,
	}
}

func isEmergencyProblem(problem string) bool {
	return strings.Contains(problem, "lek") ||
		strings.Contains(problem, "overstroming") ||
		strings.Contains(problem, "gaslucht") ||
		strings.Contains(problem, "kortsluiting") ||
		strings.Contains(problem, "geen warm water") ||
		strings.Contains(problem, "geen verwarming") ||
		strings.Contains(problem, "noodgeval") ||
		strings.Contains(problem, "urgent")
}

func applyCategoryHintBoost(hint string, plumbingScore, hvacScore, electricalScore, carpentryScore *int) {
	if hint == "" {
		return
	}

	switch hint {
	case "plumbing", "loodgieter", "water":
		(*plumbingScore)++
	case "hvac", "cv", "heating", "koeling":
		(*hvacScore)++
	case "electrical", "elektricien", "electric", "stroom":
		(*electricalScore)++
	case "carpentry", "timmerman", "hout":
		(*carpentryScore)++
	}
}

type specialistScore struct {
	name  string
	score int
}

func determineWinnerAndAlternatives(plumbingScore, hvacScore, electricalScore, carpentryScore int) (specialistScore, []string) {
	scores := []specialistScore{
		{"Loodgieter", plumbingScore},
		{"CV-monteur / HVAC specialist", hvacScore},
		{"Elektricien", electricalScore},
		{"Timmerman", carpentryScore},
	}

	sortScores(scores)
	return scores[0], collectAlternatives(scores)
}

func sortScores(scores []specialistScore) {
	for i := 0; i < len(scores)-1; i++ {
		for j := i + 1; j < len(scores); j++ {
			if scores[j].score > scores[i].score {
				scores[i], scores[j] = scores[j], scores[i]
			}
		}
	}
}

func collectAlternatives(scores []specialistScore) []string {
	if len(scores) == 0 {
		return nil
	}

	winner := scores[0]
	alternatives := []string{}
	for i := 1; i < len(scores) && i < 3; i++ {
		if scores[i].score > 0 && scores[i].score >= winner.score/2 {
			alternatives = append(alternatives, scores[i].name)
		}
	}

	return alternatives
}

func questionsForSpecialist(name string) []string {
	switch name {
	case "Loodgieter":
		return []string{
			"Is er actief lekkage? Zo ja, hoeveel water komt er uit?",
			"Waar bevindt het probleem zich precies (keuken, badkamer, etc.)?",
			"Is het drinkwater of afvoer gerelateerd?",
		}
	case "CV-monteur / HVAC specialist":
		return []string{
			"Welk merk en type CV-ketel/systeem heeft u?",
			"Geeft het apparaat foutmeldingen?",
			"Wanneer is het laatst onderhouden?",
		}
	case "Elektricien":
		return []string{
			"Valt de stroom in het hele huis uit of alleen in bepaalde groepen?",
			"Ruikt u iets branderigs of ziet u vonken?",
			"Hoe oud is uw meterkast?",
		}
	case "Timmerman":
		return []string{
			"Gaat het om reparatie of nieuw werk?",
			"Wat zijn de afmetingen (bij deuren/ramen)?",
			"Is het binnen- of buitenwerk?",
		}
	default:
		return nil
	}
}

// countKeywords counts how many keywords from the list appear in the text
func countKeywords(text string, keywords []string) int {
	count := 0
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			count++
		}
	}
	return count
}

// truncateString truncates a string to maxLen characters, adding "..." if truncated
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
