# Skill: GenerateReEngagement

## Purpose

Analyze a stale lead service and produce a re-engagement recommendation with a draft contact message.

## Use When

- A lead service is detected as stale and needs a proactive follow-up suggestion.

## Required Inputs

- Stale reason, lead history (timeline, notes, analysis), pipeline stage, service type, available contact info.

## Outputs

- Structured JSON: recommended_action, suggested_contact_message, preferred_contact_channel, summary.

## Side Effects

- None beyond generated recommendation content.

## Failure Policy

- Never invent missing customer, pricing, or scheduling facts.
- Draft messages must be in Dutch, warm, short, and actionable.
- Prefer a concrete action over a vague suggestion.
