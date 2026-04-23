# Conversation Continuity

## Purpose
Ensure natural, multi-turn WhatsApp conversations. Leverage recent context to resolve short replies seamlessly without resetting the task or repeating questions.

## Guidelines

**Timeframes & Context Expiration**
- **Active Context (< 4 hours):** Treat short replies as continuations of the most recent task or intent.
- **Stale Context (> 4 hours):** Treat short replies as a fresh intent, *unless* the user's message explicitly refers back to the earlier task.

**Interpreting Short Replies**
- **Confirmations (`ja`, `graag`, `ok`, `doe maar`):** Treat these as authorization to complete the pending action (e.g., sending a quote PDF, fetching requested details).
- **Bare Entities (Names, Dates, Pronouns):** Treat bare customer names, dates (e.g., `morgen`), or pronouns (e.g., `die van...`) as disambiguation or filtering for the current pending task. 
- Do NOT treat bare names provided mid-flow as a brand-new, unrelated search request.

**Action Bias & Minimizing Friction**
- **Do Not Re-ask Permission:** If the user previously established what they want (e.g., an address, quote, or appointment detail), execute the action immediately once the target is resolved. 
- **Tool Over Talk:** Prefer using tools to complete pending lookups. Only ask a new clarifying question if the target remains genuinely ambiguous *after* applying recent context and tool results.

## Examples of Continuity

- **Intent:** `Kan je het adres van Carola Dekker opzoeken?`
  **User Follow-up:** `Carola Dekker` (after a disambiguation prompt)
  **Action:** Continue the specific address lookup for that resolved customer.

- **Intent:** `Zoek de offerte van Carola Dekker`
  **User Follow-up:** `Die van Carola Dekker`
  **Action:** Continue resolving the pending quote lookup for Carola Dekker.

- **Intent:** `Ik heb Carola Dekker gevonden.` (Where the user previously asked for an address)
  **User Follow-up:** `Ja`
  **Action:** Fetch and provide the address directly. Do not ask what detail they want.

- **Intent:** Discussing existing appointments.
  **User Follow-up:** `Morgen`
  **Action:** Filter the active appointment search to tomorrow.

- **Intent:** Offering to send a resolved quote.
  **User Follow-up:** `Doe maar`
  **Action:** Execute the `SendQuotePDF` tool directly.

- **Intent:** An unresolved customer lookup from two days ago.
  **User Follow-up:** `Carola Dekker`
  **Action:** Treat as a fresh intent (Stale Context) and run a general `SearchLeads`.