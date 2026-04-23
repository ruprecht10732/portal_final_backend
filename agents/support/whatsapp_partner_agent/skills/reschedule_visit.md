# Skill: RescheduleVisit

## Purpose
Modify the scheduled date or time of an existing appointment for a partner or vakman based on their request.

## Use When
- The partner explicitly requests to move, postpone, or reschedule a specific visit.
- The partner mentions they "cannot make it" and provides an alternative time.

## Required Inputs
- **Appointment ID:** A specific, resolved ID for the target appointment (retrieved via tools).
- **New Timing:** A clearly defined date and/or time provided by the partner in the conversation history.
- **Context:** The partner’s current schedule to ensure there are no immediate overlaps (if tools permit).

## Execution Guidelines
1. **Identify & Resolve:** Before calling any reschedule tool, you must accurately identify which appointment the partner is referring to. Use the conversation history to link the request to a specific address or job type.
2. **Time Parsing:** Convert vague timing (e.g., "morgenmiddag," "volgende week dinsdag om 9u") into a concrete ISO-8601 format required by the tool. Assume "middag" is 13:00 unless otherwise specified.
3. **Availability Check:** If the tool supports it, verify the new slot is available.
4. **The "Vakman" Confirmation:** After rescheduling, always confirm the change in Dutch. 
   - **Tone:** Professional, direct, and peer-to-peer.
   - **Example:** *"Top, ik heb de afspraak verzet naar morgen om 14:00 uur. Je krijgt hier nog een bevestiging van."*

## Failure Policy
- **Ambiguous Appointment:** If the partner has multiple upcoming appointments and doesn't specify which one, ask: *"Voor welke klus wil je de afspraak verzetten? Die in [Stad A] of [Stad B]?"*
- **Missing Timing:** If the partner asks to reschedule but doesn't provide a new time, reply: *"Geen probleem, ik kan dit voor je aanpassen. Wanneer zou je de afspraak willen inplannen?"*
- **Past Appointments:** Do not attempt to reschedule appointments that have already been marked as completed or cancelled in the past.
- **Strict Logic:** Never execute `RescheduleVisit` based on an "implied" time. The partner must state the new timing.