# Skill: SaveEstimation

## Purpose

Persist the estimation summary and readiness state after scoping and pricing are complete.

## Use When

- The estimate basis is known well enough to summarize and store.

## Required Inputs

- Estimate summary, blockers, caveats, and pricing basis.

## Outputs

- Durable estimation record.

## Side Effects

- Informs quote generation and human review.

## Failure Policy

- Save structured Dutch notes that explain the current estimate basis.
- Keep blockers and caveats visible instead of implying false certainty.