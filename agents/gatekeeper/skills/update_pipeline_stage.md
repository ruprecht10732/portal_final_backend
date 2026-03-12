# Skill: UpdatePipelineStage

## Purpose

Move the lead service to the correct next pipeline stage once the Gatekeeper analysis is durable.

## Use When

- `SaveAnalysis` has already completed.
- The next stage is justified by trusted evidence and backend invariants.

## Required Inputs

- Target stage.
- Short reason explaining why the transition is correct now.

## Outputs

- Pipeline-stage transition or backend rejection.

## Side Effects

- Can wake downstream automation such as Calculator or Matchmaker.

## Failure Policy

- Prefer `Nurturing` when intake blockers remain.
- Request `Estimation` only when the intake is materially complete.