# Skill: LogCall

## Purpose

Turn a rough call summary into structured notes, call outcomes, lead updates, and appointment changes.

## Use When

- A real post-call summary has been provided.

## Required Inputs

- The call summary and any explicitly stated operational outcomes.

## Outputs

- Structured call result with notes and updates.

## Side Effects

- Can create notes, update lead details, update status, and mutate appointments.

## Failure Policy

- Never invent call details that were not explicitly stated.
- If booking or lead-update dependencies are unavailable, surface that limitation explicitly.