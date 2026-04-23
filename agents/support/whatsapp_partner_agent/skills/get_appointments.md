# Tool: GetAppointments

## Purpose
Retrieve a list of appointments strictly scoped to the authenticated partner's current pipeline.

## Execution Logic & Triggers
Execute this tool proactively under the following conditions:
- **Broad Overviews:** The partner asks for their schedule (e.g., "vandaag", "morgen", or a specific date range).
- **Detail Inquiries:** The partner asks when a specific visit starts, ends, or if it is still scheduled.
- **Pre-Mutation Resolution:** The partner requests a write action (e.g., cancel, update status), but the exact target is not yet resolved. Use this tool to fetch the active list and narrow down the target.

## Autonomy Rules
- **Overview Requests:** If the partner asks for an overview, fetch and list the appointments immediately. Do NOT ask which appointment they mean.
- **Single Match:** If exactly ONE upcoming appointment matches the criteria or timeframe, proceed directly with providing the requested detail or executing the requested action.

## Output Formatting
When listing appointments, provide a concise summary including:
- `Date` (Datum)
- `Start time` (Starttijd)
- `Status`
- `Location context` (Locatie) - Keep this brief if available.

## Ambiguity & Failure Policy
- **0 Matches:** If no appointments are found for the requested criteria, state this plainly: "Er staan geen afspraken gepland voor die periode."
- **Multiple Matches (Ambiguous):** If the partner asks to perform an action on an appointment but the retrieval returns multiple valid options, PAUSE. Ask exactly ONE short clarifying question to isolate the correct target.