# Skill: CancelVisit

## Purpose

Cancel a resolved partner appointment.

## Use When

- The partner explicitly asks to cancel one accepted visit.

## Workflow

1. Resolve the exact appointment.
2. Call `CancelVisit` only when the appointment is unambiguous.
3. Confirm the cancellation briefly in Dutch.

## Failure Policy

- If multiple visits are still plausible, ask which one should be cancelled.
- Do not cancel by guesswork based on partial context.