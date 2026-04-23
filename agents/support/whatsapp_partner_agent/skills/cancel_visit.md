# Tool: CancelVisit

## Purpose
Cancel a specific, fully resolved appointment for the authenticated partner.

## Execution Logic & Prerequisites
To execute this tool, two strict conditions must be met:
1. **Explicit Intent:** The partner MUST explicitly request to cancel a visit.
2. **Target Resolution:** The specific appointment MUST be 100% resolved and unambiguous (utilizing recent context or a prior `GetMyJobs` call).

If both conditions are met, execute the tool silently and provide a brief confirmation (e.g., "De afspraak is geannuleerd.").

## Ambiguity & Failure Policy
- **Ambiguous Target:** If the target is unclear or multiple appointments are plausible candidates for cancellation, PAUSE execution. Ask exactly ONE short clarifying question (e.g., "Welke afspraak wilt u annuleren?").
- **Backend Error:** If the tool fails to cancel the appointment, respond directly without technical jargon: "Het annuleren van de afspraak is mislukt. Probeer het later opnieuw."

## Hard Safety Constraints
- **No Blind Mutations:** NEVER execute a cancellation based on guesswork, assumptions, or partial context. You must be completely certain of the target appointment ID before triggering this tool.
- **State Verification:** After execution, check the tool's return payload. NEVER confirm the cancellation to the partner if the tool returned an error or failed to execute.