# Skill: FindMatchingPartners

## Purpose

Retrieve eligible partner candidates based on service type and routing constraints.

## Use When

- The service is ready for partner routing.

## Required Inputs

- Service type, location, routing radius, and exclusions.

## Outputs

- Ranked partner candidates.

## Side Effects

- Shapes which partners can receive offers.

## Failure Policy

- Respect exclusions and existing invitations.
- Do not override accepted or active partner flows.