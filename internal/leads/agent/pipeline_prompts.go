package agent

import (
	"fmt"
	"strings"
	"time"

	"portal_final_backend/internal/leads/repository"
)

func buildGatekeeperPrompt(lead repository.Lead, service repository.LeadService, notes []repository.LeadNote, intakeContext string, attachments []repository.Attachment) string {
	notesSection := buildNotesSection(notes)
	serviceNote := getValue(service.ConsumerNote)
	leadContext := buildLeadContextSection(lead, attachments)
	ruleChecks := buildRuleChecksSection(service.ServiceType, serviceNote, notes)

	return fmt.Sprintf(`You validate intake requirements.

Role: You validate intake requirements.
Goal: If valid -> set stage Ready_For_Estimator. If invalid -> set stage Nurturing.
Constraint: Do NOT calculate price. Do NOT look for partners.

Lead:
- Lead ID: %s
- Service ID: %s
- Service Type: %s
- Pipeline Stage: %s
- Created At: %s

Consumer:
- Name: %s %s
- Phone: %s
- Email: %s
- Role: %s

Address:
- %s %s, %s %s

Service Note (raw):
%s

Notes:
%s

Additional Context:
%s

Rule-based checks (heuristic):
%s

Intake Requirements:
%s

CRITICAL REQUIRED TOOL CALLS:
You MUST call BOTH SaveAnalysis AND UpdatePipelineStage in EVERY response.
SaveAnalysis MUST be called BEFORE UpdatePipelineStage.
If you skip SaveAnalysis, the lead timeline will be broken - this is NOT optional.

Instruction:
If you find high-confidence (>=90%%) errors in lead contact or address details, call UpdateLeadDetails.
Only update fields you are confident about. Include a short Dutch reason and your confidence.
0) If the service type is clearly wrong, you may call UpdateLeadServiceType ONLY when you are highly confident (>=90%%).
If you update the service type, do it BEFORE UpdatePipelineStage.
Only change the service type when there is a clear positive match to another service based on notes/service note.
Missing intake information alone is NOT a reason to switch service type.
If the intent is ambiguous, keep the current service type and move to Nurturing with a short Dutch reason.
1) Validate intake requirements for the selected service type.
2) Treat rule-based missing items as critical unless the info is clearly present elsewhere.
3) FIRST call SaveAnalysis with urgencyLevel, leadQuality, recommendedAction, preferredContactChannel, suggestedContactMessage,
   a short Dutch summary, and a Dutch list of missingInformation (empty list if nothing missing).
4) THEN call UpdatePipelineStage with stage="Ready_For_Estimator" (if all required info is present) or stage="Nurturing" (if critical info is missing).
5) Include a short reason in UpdatePipelineStage, written in Dutch.

FINAL REMINDER: You MUST output SaveAnalysis followed by UpdatePipelineStage. No exceptions.
`,
		lead.ID,
		service.ID,
		service.ServiceType,
		service.PipelineStage,
		lead.CreatedAt.Format(time.RFC3339),
		lead.ConsumerFirstName,
		lead.ConsumerLastName,
		lead.ConsumerPhone,
		getValue(lead.ConsumerEmail),
		lead.ConsumerRole,
		lead.AddressStreet,
		lead.AddressHouseNumber,
		lead.AddressZipCode,
		lead.AddressCity,
		wrapUserData(sanitizeUserInput(serviceNote, maxConsumerNote)),
		notesSection,
		leadContext,
		ruleChecks,
		intakeContext,
	)
}

func buildEstimatorPrompt(lead repository.Lead, service repository.LeadService, notes []repository.LeadNote, photoAnalysis *repository.PhotoAnalysis) string {
	notesSection := buildNotesSection(notes)
	serviceNote := getValue(service.ConsumerNote)
	photoSummary := buildPhotoSummary(photoAnalysis)

	return fmt.Sprintf(`You are a Technical Estimator.

Role: You are a Technical Estimator.
Input: Photos, Description.
Goal: Determine Scope (Small/Medium/Large). Estimate Price Range based on actual product prices.
Action: Search for products, then call SaveEstimation (metadata update). Set stage Ready_For_Partner.

Lead:
- Lead ID: %s
- Service ID: %s
- Service Type: %s
- Pipeline Stage: %s
- Created At: %s

Consumer:
- Name: %s %s
- Phone: %s
- Email: %s

Address:
- %s %s, %s %s

Service Note (raw):
%s

Notes:
%s

Photo Analysis:
%s

Instruction:
1) Identify the materials/products needed based on the service description and photos.
2) Call SearchProductMaterials with a query describing the materials needed (e.g., "isolation panels", "wood planks", "roofing materials").
   You may call SearchProductMaterials multiple times for different material categories.
	Use standard, mid-range materials unless the request explicitly calls for heavy-duty or premium.
	If multiple products are returned, prefer the most typical/affordable option for the scenario.
3) Use CalculateEstimate to compute material subtotal, labor subtotal range, and total range.
	Provide structured inputs (material items, quantities, labor hours range, hourly rate range, optional extra costs).
4) Determine scope: Small, Medium, or Large based on work complexity.
5) Call SaveEstimation with scope, priceRange (e.g. "EUR 500 - 900"), notes, and a short summary. Notes and summary must be in Dutch.
	Include the products found and their prices in the notes.
	Format notes as multiline Markdown with blank lines between sections.
	Use headings (bold labels) and bullet/numbered lists so each item is on its own line.
	Example structure:
	**Materiaalbenodigdheden:**
	1. ...
	2. ...

	**Subtotaal materiaal:** EUR ...

	**Arbeid:**
	- ...

	**Subtotaal arbeid:** EUR ...

	**Totaal geschatte kosten:** EUR ...

	**Opmerkingen:**
	- ...
6) Call UpdatePipelineStage with stage="Ready_For_Partner" and a reason in Dutch.

You MUST call SearchProductMaterials first (if available), then SaveEstimation, then UpdatePipelineStage. Respond ONLY with tool calls.
`,
		lead.ID,
		service.ID,
		service.ServiceType,
		service.PipelineStage,
		lead.CreatedAt.Format(time.RFC3339),
		lead.ConsumerFirstName,
		lead.ConsumerLastName,
		lead.ConsumerPhone,
		getValue(lead.ConsumerEmail),
		lead.AddressStreet,
		lead.AddressHouseNumber,
		lead.AddressZipCode,
		lead.AddressCity,
		wrapUserData(sanitizeUserInput(serviceNote, maxConsumerNote)),
		notesSection,
		photoSummary,
	)
}

func buildDispatcherPrompt(lead repository.Lead, service repository.LeadService, radiusKm int) string {
	return fmt.Sprintf(`You are the Fulfillment Manager.

Role: You are the Fulfillment Manager.
Action: Call FindMatchingPartners.
Logic:
- If > 0 partners: Set stage Partner_Matching. Summary: "Found X partners".
- If 0 partners: Set stage Manual_Intervention. Summary: "No partners found in range." DO NOT REJECT.

Lead:
- Lead ID: %s
- Service ID: %s
- Service Type: %s
- Pipeline Stage: %s
- Zip Code: %s

Instruction:
1) Call FindMatchingPartners with serviceType="%s", zipCode="%s", radiusKm=%d.
2) Use the number of matches to decide stage and call UpdatePipelineStage. The reason must be in Dutch.

You MUST call FindMatchingPartners and then UpdatePipelineStage. Respond ONLY with tool calls.
`,
		lead.ID,
		service.ID,
		service.ServiceType,
		service.PipelineStage,
		lead.AddressZipCode,
		service.ServiceType,
		lead.AddressZipCode,
		radiusKm,
	)
}

func buildNotesSection(notes []repository.LeadNote) string {
	meaningful := filterMeaningfulNotes(notes)
	if len(meaningful) == 0 {
		return "No notes"
	}

	var sb strings.Builder
	for _, note := range meaningful {
		body := sanitizeUserInput(note.Body, maxNoteLength)
		sb.WriteString(fmt.Sprintf("- [%s] %s: %s\n", note.Type, note.CreatedAt.Format(time.RFC3339), body))
	}
	return wrapUserData(sb.String())
}

func buildLeadContextSection(lead repository.Lead, attachments []repository.Attachment) string {
	energySummary := buildEnergySummary(lead)
	enrichmentSummary := buildEnrichmentSummary(lead)
	attachmentsSummary := buildAttachmentsSummary(attachments)

	return wrapUserData(strings.Join([]string{
		"Energy: " + energySummary,
		"Enrichment: " + enrichmentSummary,
		"Attachments: " + attachmentsSummary,
	}, "\n"))
}

func buildEnergySummary(lead repository.Lead) string {
	if lead.EnergyClass == nil && lead.EnergyIndex == nil && lead.EnergyBouwjaar == nil && lead.EnergyGebouwtype == nil {
		return "No energy label data"
	}

	parts := make([]string, 0, 4)
	if lead.EnergyClass != nil {
		parts = append(parts, "class "+*lead.EnergyClass)
	}
	if lead.EnergyIndex != nil {
		parts = append(parts, fmt.Sprintf("index %.2f", *lead.EnergyIndex))
	}
	if lead.EnergyBouwjaar != nil {
		parts = append(parts, fmt.Sprintf("build year %d", *lead.EnergyBouwjaar))
	}
	if lead.EnergyGebouwtype != nil {
		parts = append(parts, "type "+*lead.EnergyGebouwtype)
	}

	if len(parts) == 0 {
		return "No energy label data"
	}
	return strings.Join(parts, ", ")
}

func buildEnrichmentSummary(lead repository.Lead) string {
	parts := make([]string, 0, 4)
	if lead.LeadEnrichmentSource != nil {
		parts = append(parts, "source "+*lead.LeadEnrichmentSource)
	}
	if lead.LeadEnrichmentPostcode6 != nil {
		parts = append(parts, "postcode6 "+*lead.LeadEnrichmentPostcode6)
	}
	if lead.LeadEnrichmentBuurtcode != nil {
		parts = append(parts, "buurtcode "+*lead.LeadEnrichmentBuurtcode)
	}
	if lead.LeadEnrichmentConfidence != nil {
		parts = append(parts, fmt.Sprintf("confidence %.2f", *lead.LeadEnrichmentConfidence))
	}
	if len(parts) == 0 {
		return "No enrichment data"
	}
	return strings.Join(parts, ", ")
}

func buildAttachmentsSummary(attachments []repository.Attachment) string {
	if len(attachments) == 0 {
		return "No attachments"
	}

	names := make([]string, 0, 5)
	for i, att := range attachments {
		if i >= 5 {
			break
		}
		name := sanitizeUserInput(att.FileName, 80)
		names = append(names, name)
	}
	return fmt.Sprintf("%d file(s): %s", len(attachments), strings.Join(names, ", "))
}

func buildRuleChecksSection(serviceType string, serviceNote string, notes []repository.LeadNote) string {
	serviceName := strings.ToLower(serviceType)
	if !strings.Contains(serviceName, "isolat") {
		return "No rule-based checks for this service type"
	}

	combined := strings.ToLower(serviceNote + "\n" + flattenNotes(notes))
	missing := make([]string, 0, 4)
	if !containsAny(combined, []string{"spouw", "dak", "vloer", "zolder", "gevel", "muur"}) {
		missing = append(missing, "Welke delen isoleren (spouw/dak/vloer/zolder)")
	}
	if !containsAny(combined, []string{"m2", "vierkante meter", "oppervlakte"}) {
		missing = append(missing, "Geschatte oppervlakte (m2)")
	}
	if !hasYear(combined) {
		missing = append(missing, "Bouwjaar van de woning")
	}
	if !containsAny(combined, []string{"geisoleerd", "ongeisoleerd", "isolatie", "na-isolatie"}) {
		missing = append(missing, "Huidige isolatiestatus")
	}

	if len(missing) == 0 {
		return "No missing items detected"
	}

	return "Missing: " + strings.Join(missing, "; ")
}

func flattenNotes(notes []repository.LeadNote) string {
	if len(notes) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, note := range notes {
		body := sanitizeUserInput(note.Body, maxNoteLength)
		sb.WriteString(body)
		sb.WriteString(" ")
	}
	return sb.String()
}

func containsAny(text string, terms []string) bool {
	for _, term := range terms {
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
}

func hasYear(text string) bool {
	for i := 1900; i <= 2026; i++ {
		if strings.Contains(text, fmt.Sprintf("%d", i)) {
			return true
		}
	}
	return false
}

func buildPhotoSummary(photoAnalysis *repository.PhotoAnalysis) string {
	if photoAnalysis == nil {
		return "No photo analysis available."
	}

	var sb strings.Builder
	if photoAnalysis.Summary != "" {
		sb.WriteString("Summary: " + photoAnalysis.Summary + "\n")
	}
	if photoAnalysis.ScopeAssessment != "" {
		sb.WriteString("Scope: " + photoAnalysis.ScopeAssessment + "\n")
	}
	if photoAnalysis.CostIndicators != "" {
		sb.WriteString("Cost: " + photoAnalysis.CostIndicators + "\n")
	}
	if len(photoAnalysis.Observations) > 0 {
		sb.WriteString("Observations: " + strings.Join(photoAnalysis.Observations, "; ") + "\n")
	}
	if len(photoAnalysis.SafetyConcerns) > 0 {
		sb.WriteString("Safety: " + strings.Join(photoAnalysis.SafetyConcerns, "; ") + "\n")
	}
	if len(photoAnalysis.AdditionalInfo) > 0 {
		sb.WriteString("Additional: " + strings.Join(photoAnalysis.AdditionalInfo, "; ") + "\n")
	}

	return wrapUserData(sb.String())
}
