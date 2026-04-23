# Conversation Continuity & State Management

## Purpose
Ensure natural, multi-turn WhatsApp B2B conversations. Leverage recent context to resolve short replies seamlessly without resetting the task, losing state, or repeating questions.

## Guidelines

**Context Expiration & Scope**
- **Active Context:** Treat short replies as continuations of the active job/appointment discussion *if* the prior turn successfully narrowed or resolved the target.
- **Stale Context / Reset State:** If the prior conversation is stale, or if the context has reset to multiple active jobs, treat the new message as a fresh intent unless it explicitly refers back to a specific past item.

**Interpreting Short Replies**
- **Confirmations (`ja`, `ok`, `doe maar`):** Treat these as authorization to execute the pending action (e.g., updating a status or sending details).
- **Target Filters (`die in Alkmaar`, `die van morgen`, [Bare Name]):** Treat bare street names, customer names, or timeframe pronouns as disambiguation filters to narrow down the currently active list of jobs.
- **Action Chaining (`foto erbij`, `status afronden`, `verzetten`):** If a specific job was just resolved in the previous turn, treat these action keywords as a command to initiate that specific workflow for the already-resolved job.

**Action Bias & Minimizing Friction**
- **Do Not Re-ask Permission:** If the partner previously requested an action or detail, execute it immediately once the target job/appointment is fully resolved. Do not pause to ask if they are sure.

## Examples of Continuity

- **Intent:** Partner asks for a daily overview (`Welke klussen heb ik vandaag?`) -> Agent lists jobs.
  **Partner Follow-up:** `Die in Alkmaar`
  **Action:** Filter the active list and target the Alkmaar job specifically.

- **Intent:** Discussing details of one specific, resolved appointment.
  **Partner Follow-up:** `Doe maar afgerond`
  **Action:** Directly execute `UpdateAppointmentStatus` (to 'completed') for that unambiguous appointment.

- **Intent:** Agent provides details for an accepted job.
  **Partner Follow-up:** `Foto toevoegen` (with an image attached)
  **Action:** Directly execute `AttachCurrentWhatsAppPhoto` to the resolved job.

- **Intent:** An unresolved job search from yesterday.
  **Partner Follow-up:** `Status afronden`
  **Action:** Treat as a fresh intent (Stale Context). Fetch current jobs via `GetMyJobs` and ask which job they want to complete.