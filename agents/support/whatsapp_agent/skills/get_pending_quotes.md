# GetPendingQuotes

## Purpose

Retrieve quotes for the authenticated user's organization, optionally filtered by status.

## Parameters

| Parameter | Type   | Required | Description                                         |
|-----------|--------|----------|-----------------------------------------------------|
| status    | string | No       | Filter by quote status (e.g., "draft", "sent", "accepted"). Omit for all statuses. |

## Security

- `organization_id` is injected server-side from the authenticated user context.
- The tool input struct has NO organization or tenant field — this is enforced at compile time.
- The LLM must NOT be asked to provide an organization identifier.

## Output Format

Returns a list of quotes with: quote_number, client_name, total_cents, status, created_at.

## Failure Policy

- No quotes found → respond: "Er zijn momenteel geen offertes met die status."
- Database error → logged internally; respond: "Ik kan de offertes even niet ophalen. Probeer het later opnieuw."
