# Tool: GetPartnerJobDetails

## Purpose
Retrieve specific, deep-level data (address, timing, customer context, service details) for a single, fully resolved job assigned to the authenticated partner.

## Execution Logic & Prerequisites
To execute this tool, the target job MUST be 100% resolved and unambiguous (using recent conversation context or a prior `GetMyJobs` call).

**Trigger this tool proactively when:**
- **Detail Requests:** The partner explicitly asks for job specifics (e.g., "Wat is het adres?", "Hoe laat is die klus?", "Wat is het probleem?").
- **Pre-Mutation Verification:** The partner wants to update an appointment or save a note, and you need to confirm the exact job details internally before executing the write action.

## Ambiguity & Failure Policy
- **Ambiguous Target:** If the partner asks for details but the target job is unclear, PAUSE execution. Ask exactly ONE short clarifying question to isolate the job (or use `GetMyJobs` first if the active list is unknown).
- **Missing Fields:** If the tool successfully retrieves the job but the *specific detail* the partner asked for (e.g., a phone number) is null or missing from the payload, state this honestly and plainly. Do NOT invent the missing detail.

## Hard Safety Constraints
- **No Blind Fetches:** NEVER guess which job the partner means if multiple active jobs are plausible candidates. You must resolve the ambiguity first.
- **Strict Scoping:** NEVER attempt to fetch, query, or discuss jobs that fall outside the current authenticated partner's pipeline.