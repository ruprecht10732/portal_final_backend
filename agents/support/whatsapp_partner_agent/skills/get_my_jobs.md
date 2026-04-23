# Tool: GetMyJobs

## Purpose
Retrieve a list of active, accepted jobs specifically assigned to the authenticated partner.

## Execution Logic & Triggers
Execute this tool proactively under the following conditions:
- **Broad Overviews:** The partner asks for their current workload (e.g., "wat heb ik vandaag", "deze week", or a general overview).
- **Context Initialization:** The partner requests a job-specific action (e.g., adding a photo, saving a note), but the exact target job is not yet resolved. Use this tool internally as the first narrowing step.

## Output Formatting
When presenting the job list, optimize strictly for WhatsApp readability:
- Keep the summary brief and highly scannable (use short bullet points or distinct lines).
- Include only the most useful distinguishing details: `City` (Plaats), `Appointment Time` (Tijd), and `Service Type` (Dienst).

## Autonomy & Ambiguity Policy
- **0 Matches:** If no accepted jobs are found, state this plainly: "Je hebt op dit moment geen geaccepteerde klussen openstaan."
- **1 Match / Contextual Match:** If exactly ONE job matches the current conversation context, or if the partner only has one active job in total, proceed directly to the requested action or detail. Do NOT pause to ask for confirmation.
- **Multiple Matches:** If multiple jobs are active and the partner's intent is ambiguous, output the concise list and ask exactly ONE short question to isolate the target job.