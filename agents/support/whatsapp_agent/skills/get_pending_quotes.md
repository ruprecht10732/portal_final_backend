# GetQuotes

## Purpose

Retrieve quotes for the authenticated user's organization, optionally filtered by status.

## Parameters

| Parameter | Type   | Required | Description                                         |
|-----------|--------|----------|-----------------------------------------------------|
| status    | string | No       | Filter by quote status (e.g., "draft", "sent", "accepted", "goedgekeurd"). Omit for all statuses. |

## Security

- `organization_id` is injected server-side from the authenticated user context.
- The tool input struct has NO organization or tenant field — this is enforced at compile time.
- The LLM must NOT be asked to provide an organization identifier.

## Output Format

Returns a list of quotes with: quote_number, client_name, total_cents, status, created_at, and a short summary of what the quote covers.

## Failure Policy

- No quotes found → respond: "Er zijn momenteel geen offertes met die status."
- Database error → logged internally; respond: "Ik kan de offertes even niet ophalen. Probeer het later opnieuw."

## Autonomy Rules

- If the user asks for the quote of a named customer, resolve the customer and retrieve matching quotes before asking a follow-up question.
- If exactly one quote matches that customer, answer directly with the quote details.
- If multiple quotes match that same customer, list them briefly and ask which one the user means.
- If the user asks for a general quote overview, list the available quotes instead of asking which quote they mean.
