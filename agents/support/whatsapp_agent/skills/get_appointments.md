# GetAppointments

## Purpose

Retrieve upcoming appointments for the authenticated user's organization, optionally filtered by date range.

## Parameters

| Parameter | Type   | Required | Description                                   |
|-----------|--------|----------|-----------------------------------------------|
| date_from | string | No       | Start date filter (ISO 8601, e.g. "2025-01-15"). Defaults to today. |
| date_to   | string | No       | End date filter (ISO 8601, e.g. "2025-02-15"). Defaults to 30 days from now. |

## Security

- `organization_id` is injected server-side from the authenticated user context.
- The tool input struct has NO organization or tenant field — this is enforced at compile time.
- The LLM must NOT be asked to provide an organization identifier.

## Output Format

Returns a list of appointments with: title, description, start_time, end_time, status, location.

## Failure Policy

- No appointments found → respond: "Er staan geen afspraken gepland in die periode."
- Database error → logged internally; respond: "Ik kan de afspraken even niet ophalen. Probeer het later opnieuw."
