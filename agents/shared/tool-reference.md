# Tool Reference

This page complements `tool-catalog.md` with deeper contracts for the most important backend agent tools.

## SaveAnalysis

- Used by: Gatekeeper
- Purpose: Persist the structured intake analysis for a lead service.
- Inputs: summary, missing information, resolved information, extracted facts, recommended action, lead quality, confidence fields.
- Side effects: writes analysis state and can influence later stage decisions and human follow-up.
- Critical rule: must happen before `UpdatePipelineStage` for Gatekeeper flows.

## UpdateLeadDetails

- Used by: Gatekeeper, Call Logger
- Purpose: Correct explicit factual lead details such as contact and address information.
- Inputs: explicit fields only; no inferred corrections.
- Side effects: mutates durable lead profile data.
- Critical rule: only use when evidence is explicit and confidence is high.

## UpdateLeadServiceType

- Used by: Gatekeeper
- Purpose: Correct the service type when intake evidence clearly shows the current type is wrong.
- Side effects: changes service typing and affects downstream prompts, estimation guidance, and orchestration.
- Critical rule: never use missing information alone as the reason to switch service type.

## UpdatePipelineStage

- Used by: Gatekeeper, Matchmaker, Calculator fallback paths
- Purpose: Move the lead service through its operational pipeline.
- Side effects: triggers orchestrator reactions and can enqueue downstream agents.
- Critical rule: backend invariants remain authoritative; markdown guidance does not override domain rules.

## CommitScopeArtifact

- Used by: Calculator
- Purpose: Persist structured scope before pricing.
- Outputs: normalized work items, missing dimensions, completeness flags.
- Critical rule: downstream quote building must treat the artifact as the scope source of truth.

## SearchProductMaterials

- Used by: Calculator
- Purpose: Find catalog-backed products and materials for the quote.
- Critical rule: follow shared search strategy and confidence thresholds before trusting a match.

## DraftQuote

- Used by: Calculator
- Purpose: Persist a draft quote or a repaired quote.
- Critical rule: do not use when intake is still incomplete for a safe estimate.

## FindMatchingPartners

- Used by: Matchmaker
- Purpose: Retrieve eligible partner candidates for fulfillment.
- Inputs: service type, zip code, radius, exclusions.
- Critical rule: respect existing invited or active partner flows.

## CreatePartnerOffer

- Used by: Matchmaker
- Purpose: Create the actual partner offer for the chosen partner.
- Critical rule: do not move to fulfillment success without the required backend artifacts.

## SavePhotoAnalysis

- Used by: Photo Analyzer
- Purpose: Persist structured photo findings that feed Gatekeeper and Calculator.
- Critical rule: uncertain dimensions must remain uncertain; prefer onsite flags to guesses.

## FlagOnsiteMeasurement

- Used by: Photo Analyzer
- Purpose: Mark pricing-critical or verification-critical dimensions that require in-person confirmation.

## SubmitAuditResult

- Used by: Auditor
- Purpose: Persist pass/fail audit results and explicit findings for operational review.