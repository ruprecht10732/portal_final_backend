package agent

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"portal_final_backend/internal/leads/repository"
)

const noPreferencesProvided = "No preferences provided"

func buildGatekeeperPrompt(lead repository.Lead, service repository.LeadService, notes []repository.LeadNote, intakeContext string, attachments []repository.Attachment) string {
	notesSection := buildNotesSection(notes)
	serviceNote := getValue(service.ConsumerNote)
	preferencesSummary := buildPreferencesSummary(service.CustomerPreferences)
	leadContext := buildLeadContextSection(lead, attachments)
	ruleChecks := buildRuleChecksSection(service.ServiceType, serviceNote, notes)

	return fmt.Sprintf(`You validate intake requirements.

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

Preferences (from customer portal):
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
This is mandatory.

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

Respond ONLY with tool calls.
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
		preferencesSummary,
		leadContext,
		ruleChecks,
		intakeContext,
	)
}

func buildEstimatorPrompt(lead repository.Lead, service repository.LeadService, notes []repository.LeadNote, photoAnalysis *repository.PhotoAnalysis) string {
	notesSection := buildNotesSection(notes)
	serviceNote := getValue(service.ConsumerNote)
	preferencesSummary := buildPreferencesSummary(service.CustomerPreferences)
	photoSummary := buildPhotoSummary(photoAnalysis)

	return fmt.Sprintf(`You are a Technical Estimator.

Role: You are a Technical Estimator.
Input: Photos, Description.
Goal: Determine Scope, Estimate Price Range, Draft Quote, set stage to Quote_Draft.
Action: Search for products, draft a quote, call SaveEstimation (metadata update), set stage Quote_Draft.

CRITICAL ARITHMETIC RULE:
You MUST use the Calculator tool for ALL math operations. NEVER perform arithmetic in your head.
This includes area, quantity, subtotal, rounding, and unit-conversion calculations.

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

Preferences (from customer portal):
%s

Photo Analysis:
%s

Instruction:
1) Identify the materials/products needed based on the service description and photos.
2) Call SearchProductMaterials to find products. The tool uses semantic (vector) search, so craft your queries carefully:
	- Use generic category names, synonyms, and Dutch/English variants.
	- Translate consumer wording into trade + DIY/shop synonyms before searching.
	- For each material, try at least 3 variants: (a) consumer wording, (b) professional term, (c) colloquial/store term.
	- Example for "kantstukken": "dagkantafwerking", "deurlijst/chambranle", "aftimmerlat/afdeklat", "kozijnplint", "sponninglat".
	- Search broad first, then narrow.
	- Call SearchProductMaterials multiple times with DIFFERENT queries per material category.
   Always prefer the catalog collection by default.
   If the user explicitly says not to use the catalog (e.g., "ignore catalog", "no catalog", "zonder catalogus"), set useCatalog=false.
	Use standard, mid-range materials unless the request explicitly calls for heavy-duty or premium.
	If multiple products are returned, prefer the most typical/affordable option for the scenario.
	Products from the catalog will include an "id" field - remember these IDs for step 3a.

   HANDLING NO RESULTS:
   - If SearchProductMaterials returns "No relevant products found", try at least 2 more queries with different terms/synonyms.
   - If still no match, add the item as an ad-hoc line item (without catalogProductId):
     - Use your best estimate for unitPriceCents based on typical Dutch market prices for that product.
     - Keep the description factual (e.g., "RVS scharnieren (3 stuks)") — do NOT add notes like "niet in catalogus".
	  - Always check the product score/highConfidence: if highConfidence=true (score >= 0.45), use the found price without markup.
	  - Scores in 0.35-0.45 are candidate matches; verify variant/unit before using.
	  - Items with score < 0.4 may be false positives. Verify the product NAME matches what you need.
3) Use CalculateEstimate to compute material subtotal, labor subtotal range, and total range.
	Provide structured inputs (material items, quantities, labor hours range, hourly rate range, optional extra costs).
	For each material item's unitPrice, use the product's "priceEuros" value (in euros, e.g. 7.93).
	If catalog search results include a labor time, use it as the baseline for labor hours (adjust if the scope indicates otherwise).
	IMPORTANT: Before calling CalculateEstimate, use Calculator for each individual quantity or price calculation.

	LABOR RULES — check each product's "type" field:
	- type = "service" or "digital_service": the price already INCLUDES labor/installation. Do NOT add separate arbeid hours for this item.
	- type = "product" or "material": the price is material-only. You MUST add separate arbeid line items for installing/mounting these.
	Example: A "houten kozijn vervangen" at EUR 950/m2 with type "service" includes installation — no extra arbeid.
	But "RVS scharnieren" with type "material" are material-only — add arbeid for mounting them.
3a) Call DraftQuote to create a draft quote for the customer. For each item:
	- Set description using the product name. If the product has materials (the "materials" array), format as:
	  "Product name\nInclusief:\n- Material A\n- Material B"
	  Example: "Houten kozijn 120x80\nInclusief:\n- HR++ glas\n- Beslag set"
	  If materials is empty, just use the product name/description.
	- Set quantity based on the estimated need. IMPORTANT — respect the product's "unit" field:
	  The unit tells you HOW the product is sold (e.g. "per plaat van 2.5 m²", "per rol van 10 m", "per stuk", "per m²").
	  If the unit indicates a fixed size (e.g. "per plaat van 2.5 m²"), use Calculator to compute how many units are needed:
	  Call Calculator(operation="ceil_divide", a=required_area, b=unit_size) to get the quantity (always rounds up).
	  If the unit is "per m²" or "per m", use Calculator to compute the raw measurement as quantity.
	- Set unitPriceCents to the product's "priceCents" value (already in euro-cents, e.g. 793 = EUR 7.93). Do NOT use priceEuros.
	- For highConfidence=true products, use priceCents exactly (no markup).
	- Only estimate unitPriceCents when no suitable high-confidence product was found.
	- Set taxRateBps from the product's vatRateBps (e.g., 2100 for 21%%). If unknown, use 2100.
	- If the product came from SearchProductMaterials and has an "id", include it as catalogProductId.
	- catalogProductId is authoritative metadata: backend will enforce catalog unit price (and VAT) for those lines.
	- For labor or ad-hoc items NOT from the catalog, omit catalogProductId.
	- Include a notes field (in Dutch) summarizing why this quote was generated.
	Note: Catalog product documents (PDFs, specs) and terms URLs are automatically attached to the quote — you do not need to handle attachments yourself.
4) Determine scope: Small, Medium, or Large based on work complexity.
5) Call SaveEstimation with scope, priceRange (e.g. "EUR 500 - 900"), notes, and a short summary. Notes and summary must be in Dutch.
	Include the products found and their prices in the notes. If a catalog item includes labor time, mention it.
	Format notes as multiline Markdown with headings and bullet/numbered lists.
6) Call UpdatePipelineStage with stage="Quote_Draft" and a reason in Dutch.
	IMPORTANT: DO NOT use "Ready_For_Partner". We must wait for the customer to accept the quote first.

You MUST call SearchProductMaterials first (if available), then DraftQuote (if available), then SaveEstimation, then UpdatePipelineStage. Respond ONLY with tool calls.
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
		preferencesSummary,
		photoSummary,
	)
}

func buildDispatcherPrompt(lead repository.Lead, service repository.LeadService, radiusKm int, excludeIDs []uuid.UUID) string {
	exclusionTxt := ""
	if len(excludeIDs) > 0 {
		exclusionTxt = fmt.Sprintf("\nCONTEXT: The following Partner IDs have already been contacted or rejected: %v. You MUST include these in the 'excludePartnerIds' field when calling FindMatchingPartners.", excludeIDs)
	}

	return fmt.Sprintf(`You are the Fulfillment Manager.

Action: Find matches, create an offer, update the pipeline stage.%s

Logic:
- If > 0 partners found:
  1) Select the best match (e.g., closest distance).
	2) Call CreatePartnerOffer for that partner, including a short Dutch job summary.
	3) Call UpdatePipelineStage with stage="Partner_Matching" and reason "Offer verzonden naar [Partnernaam]".
- If 0 partners found:
	- Call UpdatePipelineStage with stage="Manual_Intervention" and reason "Geen partners gevonden binnen bereik." DO NOT REJECT.

Lead:
- Lead ID: %s
- Service ID: %s
- Service Type: %s
- Pipeline Stage: %s
- Zip Code: %s

Instruction:
1) Call FindMatchingPartners with serviceType="%s", zipCode="%s", radiusKm=%d (and include exclusions).
2) If matches exist, you MUST call CreatePartnerOffer BEFORE UpdatePipelineStage.
3) When calling CreatePartnerOffer, set jobSummaryShort to a short Dutch summary (max 120 chars) of what the job entails, based on service type and notes. Do NOT include exact address or personal data.
4) Use Dutch for the UpdatePipelineStage reason.

You MUST call FindMatchingPartners first. If matches exist, you MUST call CreatePartnerOffer before UpdatePipelineStage. Respond ONLY with tool calls.
`,
		exclusionTxt,
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

func buildQuoteGeneratePrompt(lead repository.Lead, service repository.LeadService, notes []repository.LeadNote, userPrompt string) string {
	notesSection := buildNotesSection(notes)
	serviceNote := getValue(service.ConsumerNote)
	preferencesSummary := buildPreferencesSummary(service.CustomerPreferences)

	return fmt.Sprintf(`You are a Quote Generator.

Role: You generate draft quotes from a user prompt using catalog product search.
Goal: Search for relevant products and create a draft quote for the customer.
Constraint: You MUST only use SearchProductMaterials, Calculator, and DraftQuote. Do NOT call any other tools.

CRITICAL ARITHMETIC RULE:
You MUST use the Calculator tool for ALL math operations. NEVER perform arithmetic in your head.
This includes area, quantity, subtotal, rounding, and unit-conversion calculations.

Lead:
- Lead ID: %s
- Service ID: %s
- Service Type: %s

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

Preferences (from customer portal):
%s

User Prompt:
%s

Instruction:
1) Read the user prompt carefully. Identify what products/materials are needed.
2) If SearchProductMaterials is available, call it to find matching products. Use semantic search tips:
   - Use generic product category names and synonyms.
	- Translate consumer wording into trade + DIY/shop synonyms before searching.
	- For each material, try at least 3 variants: (a) consumer wording, (b) professional term, (c) colloquial/store term.
	- Example for "kantstukken": "dagkantafwerking", "deurlijst/chambranle", "aftimmerlat/afdeklat", "kozijnplint", "sponninglat".
   - Mix Dutch and English terms.
   - Search broad first, then narrow.
	- Call SearchProductMaterials multiple times with DIFFERENT queries per material category.
   Always prefer the catalog collection by default.

	If SearchProductMaterials is NOT available, skip search and continue with ad-hoc DraftQuote items using best-estimate unitPriceCents.

   HANDLING NO RESULTS:
   - If SearchProductMaterials returns "No relevant products found", try at least 2 more queries with different terms/synonyms.
   - If still no match after multiple attempts, add the item as an ad-hoc line item (without catalogProductId):
     - Use your best estimate for unitPriceCents based on typical Dutch market prices.
     - Keep the description factual — do NOT add notes like "niet in catalogus".
	 - Always check the product score/highConfidence: if highConfidence=true (score >= 0.45), use the found price without markup.
	 - Scores in 0.35-0.45 are candidate matches; verify variant/unit before using.
	 - Items with score < 0.4 may be false positives. Verify the product NAME matches what you need.
3) For each product found, prepare a DraftQuote item:
   - Set description using the product name. If the product has materials (the "materials" array), format as:
     "Product name\nInclusief:\n- Material A\n- Material B"
     If materials is empty, just use the product name/description.
   - Set quantity based on the user prompt. IMPORTANT — respect the product's "unit" field:
     The unit tells you HOW the product is sold (e.g. "per plaat van 2.5 m²", "per rol van 10 m", "per stuk").
     If the unit indicates a fixed size, use Calculator to compute the quantity:
     Call Calculator(operation="ceil_divide", a=required_amount, b=unit_size) to get the quantity (always rounds up).
   - Set unitPriceCents to the product's "priceCents" value (already in euro-cents). Do NOT use priceEuros.
	- For highConfidence=true products, use priceCents exactly (no markup).
	- Only estimate unitPriceCents when no suitable high-confidence product was found.
   - Set taxRateBps from the product's vatRateBps. If unknown, use 2100.
   - If the product has an "id", include it as catalogProductId.
	- catalogProductId is authoritative metadata: backend will enforce catalog unit price (and VAT) for those lines.
   - For labor or ad-hoc items, omit catalogProductId.
   LABOR RULES — check each product's "type" field:
   - type = "service" or "digital_service": the price already INCLUDES labor. Do NOT add a separate arbeid line item.
   - type = "product" or "material": the price is material-only. Add a separate "Arbeid" line item for installing/mounting these.
   Only add arbeid for work on material/product items (e.g., mounting hinges, hanging a door).
4) Call DraftQuote with all items and a notes field (in Dutch) summarizing what was generated.
   Catalog product documents and URLs are automatically attached — you do not need to handle attachments.

If SearchProductMaterials is available, call it first. Always use Calculator for all arithmetic, then DraftQuote. Respond ONLY with tool calls.
`,
		lead.ID,
		service.ID,
		service.ServiceType,
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
		preferencesSummary,
		wrapUserData(sanitizeUserInput(userPrompt, 2000)),
	)
}

func buildPreferencesSummary(raw json.RawMessage) string {
	if len(raw) == 0 {
		return noPreferencesProvided
	}

	var prefs struct {
		Budget       string `json:"budget"`
		Timeframe    string `json:"timeframe"`
		Availability string `json:"availability"`
		ExtraNotes   string `json:"extraNotes"`
	}
	if err := json.Unmarshal(raw, &prefs); err != nil {
		return noPreferencesProvided
	}

	budget := strings.TrimSpace(prefs.Budget)
	timeframe := strings.TrimSpace(prefs.Timeframe)
	availability := strings.TrimSpace(prefs.Availability)
	extraNotes := strings.TrimSpace(prefs.ExtraNotes)

	if budget == "" && timeframe == "" && availability == "" && extraNotes == "" {
		return noPreferencesProvided
	}

	lines := []string{
		"- Budget: " + preferenceValue(budget),
		"- Timeframe: " + preferenceValue(timeframe),
		"- Availability: " + preferenceValue(availability),
		"- Extra notes: " + preferenceValue(extraNotes),
	}

	return wrapUserData(strings.Join(lines, "\n"))
}

func preferenceValue(value string) string {
	if value == "" {
		return valueNotProvided
	}
	return sanitizeUserInput(value, maxNoteLength)
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

	// New v2 fields
	if len(photoAnalysis.Measurements) > 0 {
		sb.WriteString("Measurements:\n")
		for _, m := range photoAnalysis.Measurements {
			sb.WriteString(fmt.Sprintf("  - %s: %.2f %s (%s, confidence: %s)\n", m.Description, m.Value, m.Unit, m.Type, m.Confidence))
		}
	}
	if len(photoAnalysis.NeedsOnsiteMeasurement) > 0 {
		sb.WriteString("Needs on-site measurement: " + strings.Join(photoAnalysis.NeedsOnsiteMeasurement, "; ") + "\n")
	}
	if len(photoAnalysis.Discrepancies) > 0 {
		sb.WriteString("⚠ Discrepancies (consumer claims vs photos): " + strings.Join(photoAnalysis.Discrepancies, "; ") + "\n")
	}
	if len(photoAnalysis.ExtractedText) > 0 {
		sb.WriteString("Extracted text (OCR): " + strings.Join(photoAnalysis.ExtractedText, "; ") + "\n")
	}
	if len(photoAnalysis.SuggestedSearchTerms) > 0 {
		sb.WriteString("Suggested product search terms: " + strings.Join(photoAnalysis.SuggestedSearchTerms, ", ") + "\n")
	}

	return wrapUserData(sb.String())
}
