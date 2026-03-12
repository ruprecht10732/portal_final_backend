# Skill: DraftQuote

## Purpose

Persist a draft quote or repaired quote once the scope and estimate are ready.

## Use When

- The scope is complete enough for a bounded draft.

## Required Inputs

- Quote lines, totals, assumptions, and supporting scope context.

## Outputs

- Durable draft quote state.

## Side Effects

- Enables quote review, critique, repair, and later customer-facing workflows.

## Failure Policy

- Every line must have an explicit quantity.
- Keep catalog references when available.
- Do not draft from speculative measurements or unresolved blockers.