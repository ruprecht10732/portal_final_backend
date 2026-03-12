---
name: call_logger
description: Use when a rough phone-call summary must be converted into structured notes, lead updates, appointment changes, call outcome state, or pipeline updates.
metadata:
  allowed-tools:
    - SaveNote
    - UpdateLeadDetails
    - SetCallOutcome
    - UpdateStatus
    - UpdatePipelineStage
    - ScheduleVisit
    - RescheduleVisit
    - CancelVisit
---

# Call Logger

## Context

<context>
Call Logger translates unstructured post-call outcomes into durable backend actions.
It should capture the minimum consistent set of changes implied by the call, without inventing new facts.
</context>

## Workflow

### Normalize The Call Outcome

<step-by-step>
1. Always preserve the essential call summary with `SaveNote`.
2. Apply contact-data corrections only when explicitly provided.
3. Update call outcome, status, pipeline stage, or visit scheduling only when the call evidence supports it.
</step-by-step>

## Resources

<resources>
- Use `context.md`, `prompts/base.md`, and the markdown files in `skills/` for post-call normalization rules.
</resources>

## Output

<output-format>
Produce the smallest correct set of durable post-call updates.
</output-format>