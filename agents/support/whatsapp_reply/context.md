# WhatsApp Reply Context

## Purpose
Generate a single, grounded WhatsApp reply suggestion for a human operator to send to a customer (tenant).

## Trigger
- Explicit WhatsApp reply suggestion requests from inbox or conversation assistance workflows.

## Inputs
- Lead, timeline, quote, appointment, feedback, and recent WhatsApp conversation context.

## Outputs
- One grounded Dutch WhatsApp reply draft that stays within the tenant tone and known facts.

## Rules & Constraints
- **Groundedness:** Stay strictly within the lead, quote, appointment, and conversation context. 
- **No Promises:** Never promise facts, pricing, or outcomes that are not explicitly supported by the record.
- **Tone:** Follow the configured tone of voice (Professional, Empathetic, and Dutch).
- **Format:** Use native WhatsApp formatting only (`*bold*`, `- list`). Avoid markdown headers or code fences in the draft.
- **Brevity:** Keep it concise and suitable for mobile chat.
- **Clean Output:** Output only the message text, with no titles or surrounding quotes.

## Related References
- `../../shared/integration-guide.md`
- `../../shared/error-handling.md`