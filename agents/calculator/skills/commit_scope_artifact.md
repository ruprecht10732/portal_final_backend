# Skill: CommitScopeArtifact

## Purpose

Persist the structured scope artifact that downstream estimate and quote steps depend on.

## Use When

- Scope items, quantities, and known unknowns are ready to store.

## Required Inputs

- Structured scope data and unresolved gaps.

## Outputs

- Durable scope artifact.

## Side Effects

- Becomes the scope source of truth for later pricing and drafting.

## Failure Policy

- Store explicit quantities and known unknowns.
- Do not hide uncertainty inside apparently complete scope lines.