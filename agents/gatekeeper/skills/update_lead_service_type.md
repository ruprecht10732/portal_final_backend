# Skill: UpdateLeadServiceType

## Purpose

Correct the service type when trusted evidence clearly shows the current type is wrong.

## Use When

- The runtime context proves a better service type and the backend still allows changes.

## Required Inputs

- Corrected service type.
- Explicit supporting evidence.

## Outputs

- Updated lead-service type.

## Side Effects

- Changes downstream guidance, prompts, and estimation behavior tied to the service type.

## Failure Policy

- Never use missing information as a reason to change service type.
- If evidence is mixed, preserve the current type and record uncertainty in analysis.