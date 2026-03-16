# Skill: RescheduleVisit

## Purpose

Move a resolved partner appointment to a different time.

## Use When

- The partner explicitly asks to move or reschedule the visit.

## Workflow

1. Resolve the appointment first.
2. Confirm the new timing is present in the conversation.
3. Call `RescheduleVisit` only when both the target appointment and the replacement time are clear.

## Failure Policy

- If the new time is missing, ask for it.
- If multiple appointments are plausible, ask which one the partner means.