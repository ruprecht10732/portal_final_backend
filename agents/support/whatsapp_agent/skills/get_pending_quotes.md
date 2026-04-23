# Tool: GetQuotes

## Purpose
Retrieve a list of quotes for the authenticated organization, with an optional filter for quote status.

## Parameters

| Parameter | Type   | Required | Description |
| :-------- | :----- | :------- | :---------- |
| `status`  | string | No       | Filter by quote status (e.g., "draft", "sent", "accepted", "goedgekeurd"). Omit to retrieve all statuses. |

## Security & Constraints
- **Server-Side Enforcement:** `organization_id` is automatically injected from the authenticated user context.
- **CRITICAL:** Do NOT attempt to pass, guess, or ask the user for an `organization_id` or tenant identifier. The tool input struct strictly rejects these fields.

## Autonomy & Execution Logic
- **Broad Overviews:** If the user asks for a general overview, execute the tool and list the available quotes. Do NOT ask which quote they mean.
- **Named Customers:** If the user asks for a specific customer's quote, resolve the customer and retrieve matching quotes *before* asking any clarifying questions.
  - *1 Match:* Provide the quote details directly.
  - *Multiple Matches:* List them briefly and ask exactly one follow-up question to clarify which quote the user means.

## Output Formatting
- **Included Details:** Present the quotes clearly using the available data: `quote_number`, `client_name`, `status`, `created_at`, and a short summary of what the quote covers.
- **Price Conversion:** You MUST convert `total_cents` into naturally formatted euros for the customer (e.g., convert `15000` to `EUR 150,00` or `€ 150,00`).

## Failure Policy
Use these exact Dutch phrases for errors:
- **0 Matches Found:** "Er zijn momenteel geen offertes met die status."
- **Database/System Error:** "Ik kan de offertes even niet ophalen. Probeer het later opnieuw."