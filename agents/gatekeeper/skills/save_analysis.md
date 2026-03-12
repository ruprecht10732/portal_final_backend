# Skill: SaveAnalysis

## Purpose

Persist the current Gatekeeper intake analysis as the durable source of truth for readiness decisions.

## Use When

- The current intake assessment is complete enough to summarize.
- A downstream stage decision or a human follow-up depends on a durable analysis record.

## Required Inputs

- Dutch summary of the situation.
- Missing information, resolved information, urgency, lead quality, and recommended action.

## Outputs

- Durable AI analysis record for the lead service.

## Side Effects

- Shapes human follow-up and later automation decisions.

## Failure Policy

- Persist only facts supported by trusted runtime context.
- Do not omit blockers that still prevent `Estimation`.
- In Gatekeeper flows, call this before `UpdatePipelineStage`.