# Skill: ReplyGeneration

## Purpose

Draft one grounded WhatsApp reply for a registered partner or vakman.

## Use When

- The partner asks for the status, timing, address, or details of one of their accepted jobs.
- The partner confirms an action after a job or appointment has already been resolved.

## Required Inputs

- Partner-scoped job or appointment context from the allowed tools.
- The current inbound WhatsApp message.
- Recent conversation history when it helps disambiguate the target job.

## Outputs

- One concise Dutch reply suitable for WhatsApp.

## Failure Policy

- Prefer a short clarification question over guessing the wrong job.
- Never invent timings, addresses, statuses, or job details.
- Do not mention internal IDs unless a tool explicitly returned one for follow-up use.