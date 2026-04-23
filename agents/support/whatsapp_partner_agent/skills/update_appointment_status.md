# Skill: UpdateAppointmentStatus

## Purpose
Update the lifecycle state of a specific, resolved appointment in the system to reflect the current progress or outcome reported by the partner.

## Use When
- **Completion:** The partner indicates the work is finished (e.g., "Klus is klaar," "Gereed").
- **No-Show:** The partner reports the customer was absent (e.g., "Klant niet thuis," "Niet aanwezig").
- **Cancellation:** The partner requests to abort the appointment (e.g., "Deze kan vervallen," "Annuleer deze maar").
- **Check-in/Out:** The partner provides real-time progress updates (e.g., "Ik ben er," "Ik ga nu beginnen").

## Required Inputs
- **Appointment ID:** The unique identifier for the specific visit, resolved via search tools.
- **Target Status:** The normalized status string required by the system (e.g., `COMPLETED`, `NO_SHOW`, `CANCELLED`, `IN_PROGRESS`).

## Execution Guidelines
1. **Status Mapping:** Translate the partner's natural Dutch phrasing into the specific technical status labels required by the tool. 
   - *"Ik ben klaar"* → `COMPLETED`
   - *"Niet thuis"* → `NO_SHOW`
2. **Entity Resolution:** Ensure the status is being applied to the correct appointment. If the partner has multiple active appointments, use address or time context to distinguish them.
3. **The "Vakman" Confirmation:** After the tool call succeeds, provide a short Dutch confirmation.
   - **Tone:** Direct and professional. Use "Je/Jij."
   - **Example:** *"Top, ik heb de afspraak in [Stad] op 'voltooid' gezet. Bedankt!"*
4. **Brevity:** Do not add fluff. The message should be instantly readable on a mobile notification.

## Outputs
- **Tool Call:** `UpdateAppointmentStatus` with the correct `appointment_id` and `status`.
- **Dutch Confirmation:** A single, concise WhatsApp message.

## Failure Policy
- **Unresolved Appointment:** If the partner says "Klus is klaar" but has three active jobs, ask: *"Welke klus heb je afgerond? Die in [Stad A] of [Stad B]?"*
- **Ambiguous Status:** If the intent is unclear (e.g., "Het lukt niet today"), ask for clarification: *"Zal ik de afspraak verzetten of moet ik hem als 'niet thuis' registreren?"*
- **No Hallucination:** Never update a status based on an assumption. Wait for an explicit trigger from the partner.