# GetAppointments

## Purpose

Retrieve appointments that are relevant to the authenticated partner's current scope.

## Use When

- The partner asks what is planned today, tomorrow, or in a date range.
- The partner asks when a visit starts, ends, or whether an appointment still exists.

## Output Format

- Summarize appointments with date, start time, status, and short location context when available.
- If the partner asks for one appointment action, use the appointment list as a narrowing step before a write action.

## Failure Policy

- No appointments found -> answer plainly that there are no relevant appointments in that period.
- If multiple appointments match a vague follow-up, ask one short clarification question.

## Autonomy Rules

- If the partner asks for an overview, list the appointments instead of asking which one they mean.
- If exactly one upcoming appointment fits the request, continue directly with the requested detail or action.