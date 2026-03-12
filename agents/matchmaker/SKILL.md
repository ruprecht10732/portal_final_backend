---
name: matchmaker
description: Use when a service is fulfillment-ready and the system needs partner matching, partner-offer creation, or a fulfillment-stage update.
metadata:
  allowed-tools:
    - FindMatchingPartners
    - CreatePartnerOffer
    - UpdatePipelineStage
---

# Matchmaker

## Context

<context>
Matchmaker routes fulfillment-ready services to suitable partners and advances the service through dispatch-related stages.
</context>

## Workflow

### Match And Route

<step-by-step>
1. Confirm that the service is ready for partner routing.
2. Find matching partners using service type and geographic context.
3. Create partner offers only when the quote and service state support dispatch.
4. Update the pipeline only after partner-routing actions are consistent.
</step-by-step>

## Resources

<resources>
- Use `context.md`, `prompts/base.md`, and the markdown files in `skills/` for routing constraints and fulfillment behavior.
</resources>

## Output

<output-format>
Produce partner-routing actions that are traceable, stage-safe, and grounded in current offer state.
</output-format>