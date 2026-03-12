---
name: calculator
description: Use when a service is in estimation or quote generation and the system must scope work, search materials, calculate prices, draft quotes, critique quote quality, or persist structured estimation artifacts.
metadata:
  allowed-tools:
    - Calculator
    - CalculateEstimate
    - SearchProductMaterials
    - ListCatalogGaps
    - DraftQuote
    - SaveEstimation
    - SubmitQuoteCritique
    - CommitScopeArtifact
    - AskCustomerClarification
    - UpdatePipelineStage
---

# Calculator

## Context

<context>
Calculator is the estimation and quote-building workspace used by both autonomous estimator flows and prompt-driven quote generation flows.
It must keep scope, catalog search, arithmetic, quote drafting, and critique loops internally consistent.
</context>

## Workflow

### Estimation And Quote Building

<step-by-step>
1. Derive scope from the intake and evidence before drafting prices.
2. Use `SearchProductMaterials` and `ListCatalogGaps` to ground material selection.
3. Use `Calculator` for arithmetic and `CalculateEstimate` for structured totals.
4. Use `DraftQuote` only after quantities, units, and prices are justified.
5. Persist the resulting state with `SaveEstimation`, `CommitScopeArtifact`, or `SubmitQuoteCritique` when the active flow requires it.
</step-by-step>

### Clarification And Stage Safety

<step-by-step>
1. Use `AskCustomerClarification` when scope cannot be safely completed.
2. Use `UpdatePipelineStage` only when the estimation workflow explicitly warrants a stage move.
</step-by-step>

## Resources

<resources>
- Use `context.md`, `INTEGRATION.md`, prompt files in `prompts/`, and markdown files in `skills/` as the detailed scope and quote playbook.
</resources>

## Output

<output-format>
Produce scope, pricing, and quote actions that are numerically correct, catalog-aware, and safe for downstream approval flows.
</output-format>