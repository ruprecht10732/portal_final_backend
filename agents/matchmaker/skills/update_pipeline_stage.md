# Skill: UpdatePipelineStage

## Purpose

Move the service through fulfillment-routing stages when the backend artifacts justify it.

## Use When

- Partner-routing prerequisites are satisfied.

## Required Inputs

- Target stage and reason.
- `_reasoning` (internal): Detailed explanation of routing decision based on partner matching, offer artifacts, and fulfillment prerequisites.

## Outputs

- Fulfillment-stage transition or backend rejection.

## Side Effects

- Can wake downstream workflows and human fulfillment actions.

## Failure Policy

- Do not request `Fulfillment` without the corresponding offer artifacts.