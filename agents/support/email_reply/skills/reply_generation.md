# Skill: EmailReplyGeneration

## Purpose
Draft one grounded, professional email reply suggestion for a human operator to review and send. The goal is to provide a high-quality, tenant-tone-aligned response that addresses the customer's needs based on existing system data.

## Use When
- An operator requires assistance drafting a response to an inbound customer or lead email.
- Communication context (Leads, Services, Quotes, or Appointments) is available to provide a fact-based answer.

## Required Inputs
- **Context:** Detailed lead, service, quote, and appointment history.
- **Inbound Content:** The full text of the incoming email thread.
- **Timeline:** Current status of the service (e.g., "Quote sent," "Appointment scheduled").

## Execution Guidelines
1.  **Logical Structure:** Every draft should include:
    * **Subject Line:** A clear, relevant subject (keep the existing thread's subject if applicable).
    * **Salutation:** Professional Dutch greeting (e.g., *"Beste [Naam],"* or *"Geachte heer/mevrouw [Achternaam],"* depending on history).
    * **The Body:** Directly address the customer's query using provided data.
    * **Sign-off:** A professional closing (e.g., *"Met vriendelijke groet,"* followed by the company name/operator placeholder).
2.  **Tenant-Tone Alignment:**
    * **Language:** Dutch (NL).
    * **Style:** Professional, empathetic, and clear. Avoid overly technical jargon unless the customer is a professional.
    * **Formality:** Default to "U" unless the conversation history shows a clear transition to "Je/Jij."
3.  **Data Integrity:** * Reference specific dates, times, and service types from the tools.
    * If a specific piece of information is missing but necessary (e.g., a specific time for an appointment), use a clear placeholder like `[tijdstip]` so the operator knows what to fill in.

## Outputs
- **Format:** One complete email draft in a structured text format.
- **Constraint:** Do not include internal reasoning or metadata in the draft itself.

## Failure Policy
- **No Promises:** Never promise a specific outcome, discount, or schedule that isn't already confirmed in the system context.
- **Ambiguity:** If the customer's intent is completely unclear, draft a polite "clarification" email asking for more details.
- **Safety:** Do not include internal IDs, backend comments, or sensitive lead data not meant for customer eyes.

---

### Key Improvements Made:
* **Structure:** Explicitly defined the requirements for Subject, Salutation, and Sign-off.
* **Placeholder Logic:** Unlike WhatsApp (where we ask questions), email assistants should provide a template with placeholders `[...]` for human operators to finalize.
* **Formality Level:** Established "U" as the default for email, contrasting with the "Je/Jij" used for vakmannen.
* **Tone Alignment:** Emphasized empathy and clarity, which is standard for customer-facing home services.