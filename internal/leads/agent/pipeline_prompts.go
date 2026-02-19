package agent

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"portal_final_backend/internal/leads/repository"
)

const noPreferencesProvided = "No preferences provided"

const (
	maxGatekeeperServiceNoteChars = 2000
	maxGatekeeperNotesChars       = 3000
	maxGatekeeperPreferencesChars = 1200
	maxGatekeeperPhotoChars       = 2500
	maxGatekeeperLeadCtxChars     = 1200
	maxGatekeeperIntakeChars      = 3000

	maxEstimatorServiceNoteChars = 2000
	maxEstimatorNotesChars       = 3000
	maxEstimatorPreferencesChars = 1200
	maxEstimatorPhotoChars       = 3500

	maxQuoteServiceNoteChars = 2000
	maxQuoteNotesChars       = 2500
	maxQuotePreferencesChars = 1200
	maxQuoteUserPromptChars  = 1500
)

const extraNotesLinePrefix = "\n- Extra notes: "

func buildGatekeeperPrompt(lead repository.Lead, service repository.LeadService, notes []repository.LeadNote, intakeContext string, attachments []repository.Attachment, photoAnalysis *repository.PhotoAnalysis) string {
	notesSection := buildNotesSection(notes, maxGatekeeperNotesChars)
	serviceNote := getValue(service.ConsumerNote)
	preferencesSummary := buildPreferencesSummary(service.CustomerPreferences, maxGatekeeperPreferencesChars)
	leadContext := truncatePromptSection(buildLeadContextSection(lead, attachments), maxGatekeeperLeadCtxChars)
	photoSummary := truncatePromptSection(buildPhotoSummary(photoAnalysis), maxGatekeeperPhotoChars)
	serviceNoteSummary := truncatePromptSection(wrapUserData(sanitizeUserInput(serviceNote, maxConsumerNote)), maxGatekeeperServiceNoteChars)
	intakeContextSummary := truncatePromptSection(intakeContext, maxGatekeeperIntakeChars)

	return fmt.Sprintf(`You validate intake requirements.

Goal: If valid -> set stage Estimation. If invalid -> set stage Nurturing.
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

Photo Analysis (AI visual inspection):
%s

Additional Context:
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
0) Service type stability rule (NO flip-flopping):
	- Do NOT change the service type just because new notes/attachments arrive.
	- Only call UpdateLeadServiceType when the current service is still in pipeline stage "Triage" AND you are highly confident (>=90%%) there is a clear positive match to another active service type.
	- Missing intake information alone is NOT a reason to switch service type.
	- If the intent is ambiguous, keep the current service type and move to Nurturing with a short Dutch reason.
	- If you update the service type, do it BEFORE UpdatePipelineStage.
1) Validate intake requirements for the selected service type.
2) Treat missing required items as critical unless the info is clearly present elsewhere (e.g. in Photo Analysis).
3) FIRST call SaveAnalysis with urgencyLevel, leadQuality, recommendedAction, preferredContactChannel, suggestedContactMessage,
   a short Dutch summary, and a Dutch list of missingInformation (empty list if nothing missing).
4) THEN call UpdatePipelineStage with stage="Estimation" (if all required info is present) or stage="Nurturing" (if critical info is missing).
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
		serviceNoteSummary,
		notesSection,
		preferencesSummary,
		photoSummary,
		leadContext,
		intakeContextSummary,
	)
}

func buildEstimatorPrompt(lead repository.Lead, service repository.LeadService, notes []repository.LeadNote, photoAnalysis *repository.PhotoAnalysis, estimationContext string) string {
	notesSection := buildNotesSection(notes, maxEstimatorNotesChars)
	serviceNote := getValue(service.ConsumerNote)
	preferencesSummary := buildPreferencesSummary(service.CustomerPreferences, maxEstimatorPreferencesChars)
	photoSummary := truncatePromptSection(buildPhotoSummary(photoAnalysis), maxEstimatorPhotoChars)
	serviceNoteSummary := truncatePromptSection(wrapUserData(sanitizeUserInput(serviceNote, maxConsumerNote)), maxEstimatorServiceNoteChars)
	estimationContextSummary := truncatePromptSection(estimationContext, maxGatekeeperIntakeChars)

	return fmt.Sprintf(`You are a Technical Estimator.

Role: You are a Technical Estimator.
Input: Photos, Description.
Goal: Determine Scope, Estimate Price Range, Draft Quote. Keep stage as Estimation.
Action: Search for products, draft a quote, call SaveEstimation (metadata update).

CRITICAL ARITHMETIC RULE:
You MUST use the Calculator tool for ALL math operations. NEVER perform arithmetic in your head.
This includes area, quantity, subtotal, rounding, and unit-conversion calculations.

CRITICAL UNIT RULE (EUROS vs CENTS):
- CalculateEstimate expects unitPrice in EUROS (float), e.g. 7.93.
- DraftQuote expects unitPriceCents in EURO-CENTS (int), e.g. 793.
- NEVER pass cents into CalculateEstimate.
- If you only have priceCents, convert to euros using Calculator(operation="divide", a=priceCents, b=100).
- Do NOT invent markups/margins. Pass through catalog prices; pricing normalization is handled by backend business rules.

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

Estimation Guidelines:
%s

Catalog Improvement Signals:
- You MUST call ListCatalogGaps once at the start to see frequent catalog search misses and frequently used ad-hoc quote items.
- ListCatalogGaps defaults use the organization's configured catalog gap period + threshold.

Instruction:
0) Call ListCatalogGaps once at the start. Use it to:
	- Reuse consistent wording for ad-hoc items that show up frequently.
	- Mention up to 3 top gaps in your SaveEstimation notes as suggestions for catalog expansion.
1) Identify the materials/products needed based on the service description and photos.
2) Call SearchProductMaterials to find products. The tool uses semantic (vector) search, so craft your queries carefully:
	- Use generic category names, synonyms, and Dutch/English variants.
	- Translate consumer wording into trade + DIY/shop synonyms before searching.
	- Search broad first, then narrow.
	- Call SearchProductMaterials multiple times with DIFFERENT queries per material category.
   Always prefer the catalog collection by default.
   If the user explicitly says not to use the catalog (e.g., "ignore catalog", "no catalog", "zonder catalogus"), set useCatalog=false.
	Use standard, mid-range materials unless the request explicitly calls for heavy-duty or premium.
	CATALOG PRIORITY: When reviewing results, always prefer a catalog item (has an "id" field) over a reference item (no "id" field) for the same product need — even if the reference item has a slightly higher score. Reference items only appear when no catalog match exists; treat them as a last resort.
	If multiple catalog products are returned, prefer the most typical/affordable option for the scenario.
	Products from the catalog will include an "id" field - remember these IDs for step 3a.

   HANDLING NO RESULTS:
   - If SearchProductMaterials returns "No relevant products found", try at least 2 more queries with different terms/synonyms.
   - If still no match, add the item as an ad-hoc line item (without catalogProductId):
     - Use your best estimate for unitPriceCents based on typical Dutch market prices for that product.
     - Keep the description factual (e.g., "RVS scharnieren (3 stuks)") — do NOT add notes like "niet in catalogus".
	  - Always check the product score/highConfidence: if highConfidence=true (score >= 0.45), you can trust the found catalog price.
	  - Scores in 0.35-0.45 are candidate matches; verify variant/unit before using.
	  - Items with score < 0.4 may be false positives. Verify the product NAME matches what you need.
	  - IMPORTANT: Do NOT decide margins/markup here. Use catalog prices as provided; commercial logic lives outside the LLM.
3) Use the Estimation Guidelines to ensure your quote is complete and includes all necessary layers, materials, and labor steps.
4) Use CalculateEstimate to compute material subtotal, labor subtotal range, and total range.
	Provide structured inputs (material items, quantities, labor hours range, hourly rate range, optional extra costs).
	For each material item's unitPrice (EUROS):
	- Prefer converting from the product's priceCents: call Calculator(operation="divide", a=priceCents, b=100).
	- Use that exact result as unitPrice.
	If catalog search results include a labor time, use it as the baseline for labor hours (adjust if the scope indicates otherwise).
	IMPORTANT: Before calling CalculateEstimate, use Calculator ONLY for unit conversions and quantity calculations (ceil_divide, round, etc.).
	DO NOT pre-calculate subtotals (unitPrice × quantity) or labor totals (hours × rate). Provide raw inputs only; CalculateEstimate must do all multiplication.

	LABOR RULES — check each product's "type" field:
	- type = "service" or "digital_service": the price already INCLUDES labor/installation. Do NOT add separate arbeid hours for this item.
	- type = "product" or "material": the price is material-only. You MUST add separate arbeid line items for installing/mounting these.
	Example: A "houten kozijn vervangen" at EUR 950/m2 with type "service" includes installation — no extra arbeid.
	But "RVS scharnieren" with type "material" are material-only — add arbeid for mounting them.
4a) Call DraftQuote to create a draft quote for the customer. For each item:
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
	  NEVER set unitPriceCents to 0 for a real product — if priceCents is 0, use your best Dutch market estimate instead. If the product has an "id" (catalog item), still include catalogProductId regardless; the backend will override with the correct database price.
	- Only estimate unitPriceCents when no suitable product was found (ad-hoc items without catalogProductId).
	- Set taxRateBps from the product's vatRateBps (e.g., 2100 for 21%%). If unknown, use 2100.
	- If the product came from SearchProductMaterials and has an "id", include it as catalogProductId.
	- catalogProductId is authoritative metadata: backend will enforce catalog unit price (and VAT) for those lines.
	- For labor or ad-hoc items NOT from the catalog, omit catalogProductId.
	- Include a notes field (in Dutch) summarizing why this quote was generated.
	Note: Catalog product documents (PDFs, specs) and terms URLs are automatically attached to the quote — you do not need to handle attachments yourself.
5) Determine scope: Small, Medium, or Large based on work complexity.
6) Call SaveEstimation with scope, priceRange (e.g. "EUR 500 - 900"), notes, and a short summary. Notes and summary must be in Dutch.
	Include the products found and their prices in the notes. If a catalog item includes labor time, mention it.
	Format notes as multiline Markdown with headings and bullet/numbered lists.
7) Call UpdatePipelineStage with stage="Estimation" and a reason in Dutch.
	IMPORTANT: DO NOT move to "Fulfillment". We must wait for the customer to accept the quote first.

Tool-call order:
- If SearchProductMaterials is available, call it first.
- Then (recommended) call CalculateEstimate, then DraftQuote, then SaveEstimation, then UpdatePipelineStage.
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
		lead.AddressStreet,
		lead.AddressHouseNumber,
		lead.AddressZipCode,
		lead.AddressCity,
		serviceNoteSummary,
		notesSection,
		preferencesSummary,
		photoSummary,
		estimationContextSummary,
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
	Selection strategy (do not optimize only for distance):
	- Prefer partners with lower rejectedOffers30d.
	- If a partner has many open offers (openOffers30d), treat as capacity risk.
	- Use distance as a tie-breaker after responsiveness/capacity signals.
	- Practical rule of thumb: if the closest partner has rejectedOffers30d >= 5 and there is another partner within +5km, pick the alternative.

  1) Select the best match using the strategy above.
	2) Call CreatePartnerOffer for that partner, including a short Dutch job summary.
	3) Call UpdatePipelineStage with stage="Fulfillment" and reason "Offer verzonden naar [Partnernaam]".
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

func buildNotesSection(notes []repository.LeadNote, maxChars int) string {
	if len(notes) == 0 {
		return "No notes"
	}

	sorted := sortNotesForPrompt(notes)
	contentBudget := resolveNotesContentBudget(maxChars)
	content := renderNotesWithinBudget(sorted, contentBudget)
	if strings.TrimSpace(content) == "" {
		return "No notes"
	}
	return wrapUserData(content)
}

type scoredNote struct {
	n repository.LeadNote
	p int
}

func scoreNoteForPrompt(n repository.LeadNote) int {
	nt := strings.ToLower(strings.TrimSpace(n.Type))
	body := strings.ToLower(n.Body)

	// Lowest priority: system/log style notes.
	if nt == "system" || strings.Contains(nt, "system") || strings.Contains(nt, "log") {
		return 100
	}

	// Highest priority: explicit contact and constraint notes.
	if strings.Contains(nt, "call") || strings.Contains(nt, "phone") || strings.Contains(nt, "contact") || strings.Contains(nt, "email") || strings.Contains(nt, "sms") || strings.Contains(nt, "whatsapp") {
		return 0
	}
	if strings.Contains(body, "bel") || strings.Contains(body, "call") || strings.Contains(body, "contact") || strings.Contains(body, "na ") || strings.Contains(body, "after") || strings.Contains(body, "alleen") || strings.Contains(body, "only") || strings.Contains(body, "allerg") {
		return 10
	}

	return 50
}

func sortNotesForPrompt(notes []repository.LeadNote) []repository.LeadNote {
	// Truncation blindness guard:
	// Prefer contact/call/email constraints and operator notes over system logs.
	candidates := make([]scoredNote, 0, len(notes))
	for _, n := range notes {
		candidates = append(candidates, scoredNote{n: n, p: scoreNoteForPrompt(n)})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].p != candidates[j].p {
			return candidates[i].p < candidates[j].p
		}
		return candidates[i].n.CreatedAt.After(candidates[j].n.CreatedAt)
	})

	sorted := make([]repository.LeadNote, 0, len(candidates))
	for _, c := range candidates {
		sorted = append(sorted, c.n)
	}
	return sorted
}

func resolveNotesContentBudget(maxChars int) int {
	if maxChars <= 0 {
		maxChars = maxEstimatorNotesChars
	}
	// Headroom because wrapUserData XML-escapes content and adds wrapper tags.
	contentBudget := maxChars - 64
	if contentBudget < 200 {
		contentBudget = maxChars
	}
	return contentBudget
}

func renderNotesWithinBudget(notes []repository.LeadNote, contentBudget int) string {
	var sb strings.Builder
	for _, n := range notes {
		body := sanitizeUserInput(n.Body, maxNoteLength)
		prefix := fmt.Sprintf("- [%s] %s: ", n.Type, n.CreatedAt.Format(time.RFC3339))
		line := prefix + body + "\n"

		if len([]rune(sb.String()+line)) <= contentBudget {
			sb.WriteString(line)
			continue
		}

		remaining := contentBudget - len([]rune(sb.String()+prefix+"\n"))
		if remaining <= 0 {
			break
		}
		truncated := strings.TrimSpace(truncateRunes(body, remaining))
		if truncated == "" {
			break
		}
		sb.WriteString(prefix + truncated + "... [afgekapt]\n")
		break
	}
	return sb.String()
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

func buildQuoteGeneratePrompt(lead repository.Lead, service repository.LeadService, notes []repository.LeadNote, userPrompt string, estimationContext string) string {
	notesSection := buildNotesSection(notes, maxQuoteNotesChars)
	serviceNote := getValue(service.ConsumerNote)
	preferencesSummary := buildPreferencesSummary(service.CustomerPreferences, maxQuotePreferencesChars)
	serviceNoteSummary := truncatePromptSection(wrapUserData(sanitizeUserInput(serviceNote, maxConsumerNote)), maxQuoteServiceNoteChars)
	userPromptSummary := truncatePromptSection(wrapUserData(sanitizeUserInput(userPrompt, 2000)), maxQuoteUserPromptChars)
	estimationContextSummary := truncatePromptSection(estimationContext, maxGatekeeperIntakeChars)

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

Estimation Guidelines:
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
	CATALOG PRIORITY: When reviewing results, always prefer a catalog item (has an "id" field) over a reference item (no "id" field) for the same product need — even if the reference item has a slightly higher score. Reference items only appear when no catalog match exists; treat them as a last resort.

	If SearchProductMaterials is NOT available, skip search and continue with ad-hoc DraftQuote items using best-estimate unitPriceCents.

   HANDLING NO RESULTS:
   - If SearchProductMaterials returns "No relevant products found", try at least 2 more queries with different terms/synonyms.
   - If still no match after multiple attempts, add the item as an ad-hoc line item (without catalogProductId):
     - Use your best estimate for unitPriceCents based on typical Dutch market prices.
     - Keep the description factual — do NOT add notes like "niet in catalogus".
	 - Always check the product score/highConfidence: if highConfidence=true (score >= 0.45), you can trust the found catalog price.
	 - Scores in 0.35-0.45 are candidate matches; verify variant/unit before using.
	 - Items with score < 0.4 may be false positives. Verify the product NAME matches what you need.
3) Use the Estimation Guidelines to ensure your quote is complete and includes all necessary layers, materials, and labor steps.
4) For each product found, prepare a DraftQuote item:
   - Set description using the product name. If the product has materials (the "materials" array), format as:
     "Product name\nInclusief:\n- Material A\n- Material B"
     If materials is empty, just use the product name/description.
   - Set quantity based on the user prompt. IMPORTANT — respect the product's "unit" field:
     The unit tells you HOW the product is sold (e.g. "per plaat van 2.5 m²", "per rol van 10 m", "per stuk").
     If the unit indicates a fixed size, use Calculator to compute the quantity:
     Call Calculator(operation="ceil_divide", a=required_amount, b=unit_size) to get the quantity (always rounds up).
   - Set unitPriceCents to the product's "priceCents" value (already in euro-cents). Do NOT use priceEuros.
	  NEVER set unitPriceCents to 0 for a real product — if priceCents is 0, use your best Dutch market estimate instead. If the product has an "id" (catalog item), still include catalogProductId regardless; the backend will override with the correct database price.
	- If highConfidence=true, you can trust the found catalog priceCents.
	- Only estimate unitPriceCents when no suitable high-confidence product was found (ad-hoc items without catalogProductId).
   - Set taxRateBps from the product's vatRateBps. If unknown, use 2100.
   - If the product has an "id", include it as catalogProductId.
	- catalogProductId is authoritative metadata: backend will enforce catalog unit price (and VAT) for those lines.
   - For labor or ad-hoc items, omit catalogProductId.
   LABOR RULES — check each product's "type" field:
   - type = "service" or "digital_service": the price already INCLUDES labor. Do NOT add a separate arbeid line item.
   - type = "product" or "material": the price is material-only. Add a separate "Arbeid" line item for installing/mounting these.
   Only add arbeid for work on material/product items (e.g., mounting hinges, hanging a door).
5) Call DraftQuote with all items and a notes field (in Dutch) summarizing what was generated.
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
		serviceNoteSummary,
		notesSection,
		preferencesSummary,
		estimationContextSummary,
		userPromptSummary,
	)
}

func truncatePromptSection(section string, maxChars int) string {
	if maxChars <= 0 {
		return section
	}
	runes := []rune(section)
	if len(runes) <= maxChars {
		return section
	}
	suffix := "\n...[truncated for token budget]"
	suffixRunes := []rune(suffix)
	keep := maxChars - len(suffixRunes)
	if keep <= 0 {
		return string(runes[:maxChars])
	}
	return string(runes[:keep]) + suffix
}

func buildPreferencesSummary(raw json.RawMessage, maxChars int) string {
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

	// Truncation blindness guard: keep budget/timeframe/availability visible and
	// truncate extra notes first if we exceed the prompt budget.
	baseLines := []string{
		"- Budget: " + preferenceValue(budget),
		"- Timeframe: " + preferenceValue(timeframe),
		"- Availability: " + preferenceValue(availability),
	}
	content := strings.Join(baseLines, "\n")
	if extraNotes != "" {
		content = content + extraNotesLinePrefix + preferenceValue(extraNotes)
	}

	wrapped := wrapUserData(content)
	if maxChars > 0 && len([]rune(wrapped)) > maxChars {
		if extraNotes != "" {
			prefixWrapped := wrapUserData(strings.Join(baseLines, "\n") + extraNotesLinePrefix)
			available := maxChars - len([]rune(prefixWrapped))
			if available > 0 {
				trimmedExtra := truncateRunes(preferenceValue(extraNotes), available)
				content = strings.Join(baseLines, "\n") + extraNotesLinePrefix + trimmedExtra + "... [afgekapt]"
				wrapped = wrapUserData(content)
			}
		}
		if len([]rune(wrapped)) > maxChars {
			wrapped = truncatePromptSection(wrapped, maxChars)
		}
	}

	return wrapped
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
