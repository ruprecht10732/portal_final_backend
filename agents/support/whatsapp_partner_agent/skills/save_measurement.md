# Skill: SaveMeasurement

## Purpose
Capture and store technical field data, physical dimensions, or site-specific logistical notes provided by the partner for a specific appointment record.

## Use When
- **Technical Data:** The partner provides physical dimensions (e.g., "200x150cm", "30m2", "80mm").
- **Site Logistics:** The partner shares critical access or environment notes (e.g., "ladder nodig," "parkeren op de oprit," "sleutel ligt onder de mat").
- **Reporting:** The partner provides specific details that need to be included in the final visit report for the customer or office.

## Required Inputs
- **Appointment ID:** A specific, resolved ID for the target job (retrieved via tools).
- **Data Payload:** The raw measurements, technical values, or accessibility notes extracted from the partner's message.

## Execution Guidelines
1. **Precision Extraction:** Capture the exact numbers and units provided. If the partner says "twee bij drie," record it exactly; do not guess the unit (cm vs. m) unless it is explicitly stated or contextually certain.
2. **No Normalization:** Do not "clean up" or paraphrase technical notes in a way that might change their meaning. The "vakman's" original technical phrasing is often more accurate for the backend.
3. **The "Vakman" Confirmation:** Confirm the receipt of data in Dutch. Keep it direct.
   - *Example:* "Top, ik heb de maten (200x150) en de opmerking over de ladder opgeslagen bij deze klus."
4. **Contextual Awareness:** Ensure the data is attached to the *resolved* appointment from the current conversation history.

## Outputs
- **Tool Call:** `SaveMeasurement` (or the designated tool) with the Appointment ID and the extracted data.
- **Dutch Confirmation:** A single, concise WhatsApp message confirming exactly what was saved.

## Failure Policy
- **Missing Data:** If the partner mentions measurements but doesn't provide them, ask: *"Ik heb de klus gevonden. Wat zijn de exacte maten? Dan zet ik ze er direct in."*
- **Ambiguous Target:** If the partner provides measurements but has multiple active/pending jobs, ask: *"Voor welke klus zijn deze maten? Die in [Stad A] of [Stad B]?"*
- **Strict Grounding:** Do not invent measurement units. If only numbers are provided without units, save the numbers exactly as sent.