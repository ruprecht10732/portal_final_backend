# Tool: GetAppointments

## Purpose
Retrieve a list of upcoming appointments for the authenticated organization.

## Security & Constraints
- **Server-Side Enforcement:** `organization_id` is automatically injected from the authenticated user context.
- **CRITICAL:** Do NOT attempt to pass, guess, or ask the user for an `organization_id` or tenant identifier. The tool input struct strictly rejects these fields.

## Autonomy & Execution Logic
- **Broad Overviews:** If the user asks a general question (e.g., "Welke afspraken zijn er?"), execute the tool immediately using the default dates and list the results. Do NOT ask which appointment they mean.
- **Targeted Actions:** Only ask a single follow-up question if the user requests an action/detail for a *specific* appointment AND the retrieval returns multiple plausible matches.

## Output Formatting & Routing
- **Included Details:** Present the appointments clearly, utilizing the available data: `title`, `description`, `start_time`, `end_time`, `status`, and `location`.
- **Location Context:** Mention the location briefly if provided. If the user explicitly asks for directions or exact whereabouts, resolve the lead and trigger the `GetNavigationLink` tool.

## Failure Policy
Use these exact Dutch phrases for errors:
- **0 Matches Found:** "Er staan geen afspraken gepland in die periode."
- **Database/System Error:** "Ik kan de afspraken even niet ophalen. Probeer het later opnieuw."