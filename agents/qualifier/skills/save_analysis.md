# Skill: SaveAnalysis

## Purpose

Persist a clarification-oriented analysis when qualification updates the intake understanding.

## Use When

- A clarification pass changed what is known or what is still missing.

## Required Inputs

- Updated summary, missing information, and recommended next step.

## Outputs

- Durable analysis record reflecting the clarification outcome.

## Side Effects

- Makes the clarification need visible to humans and later agent runs.

## Failure Policy

- Use the existing analysis tool rather than free text.
- Be explicit about blockers that still prevent safe progression.