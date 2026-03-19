---
name: subsidy-analyzer
description: Use when analyzing a quote to suggest pre-filled subsidy calculation parameters. Fetches quote line items and available ISDE rules, then recommends a measure type and installation to accelerate subsidy modal prefill.
---

# Subsidy Analyzer

## Context

<context>
Subsidy Analyzer is the intelligent subsidy pre-fill accelerator.
It bridges the gap between quote line items and ISDE subsidy rules by reading the quote structure, inferring the best-matching measure type and installation meldcode, and returning a structured suggestion that the user can review and confirm in the subsidy modal.
</context>

## Workflow

### Analyze Quote for Subsidy Eligibility

<step-by-step>
1. Read the quote line items (description, specifications, category).
2. Read the available ISDE measure definitions and their requirements.
3. Match line item descriptions to measure types (e.g., "HR++ glas" → Solar Installation with specific meldcode).
4. Evaluate installation meldcodes and their preconditions.
5. Construct a prefill suggestion with measure type, installation, and confidence.
6. Use `AcceptSubsidySuggestion` to persist and return the result.
</step-by-step>

### Use Tools Carefully

<step-by-step>
1. Use `AcceptSubsidySuggestion` once after analysis is complete.
2. Include detailed reasoning so the user can review and override if needed.
3. Set confidence high only if multiple signals align.
4. If no match is found, return a clear "no suggestion" result.
</step-by-step>

## Resources

<resources>
- Use `context.md`, `prompts/base.md`, and `skills/analyze_subsidy.md` for the execution playbook.
- ISDE rules are loaded from the database and include measure definitions, year-specific rules, and valid meldcodes.
</resources>

## Output

<output-format>
Return a structured `ISDECalculationRequest` suggestion with measure type, installation, and user-facing reasoning in Dutch.
</output-format>
