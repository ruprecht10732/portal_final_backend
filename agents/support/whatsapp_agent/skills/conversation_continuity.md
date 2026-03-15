# Conversation Continuity

## Purpose

Help the WhatsApp agent continue a multi-turn customer conversation naturally without resetting the task after every short reply.

## Guidelines

- Treat short follow-ups like `ja`, `graag`, `ok`, or a bare customer name as continuations of the previous task when the prior user turn already made the requested field clear.
- If the previous relevant turn is old, for example more than 4 hours, treat a short new message as a fresh intent unless it clearly refers back to the earlier task.
- If the user already asked for a specific field such as address, phone number, e-mail, status, quote, or appointment details, do not ask for permission again once the correct customer or quote is resolved.
- When a customer name appears after a prior lookup request, interpret that as disambiguation or confirmation, not as a brand-new request.
- When a customer name appears after a prior quote request, continue the pending quote lookup for that customer.
- Prefer completing the pending lookup with tools over asking an extra clarification question.
- Only ask a new question when the target is still genuinely ambiguous after using the available context and tools.

## Examples

- `Zoek Carola Dekker` -> customer name search.
- `Kan je het adres van Carola Dekker opzoeken?` -> concrete address lookup.
- `Carola Dekker` after that -> continue the address lookup for that customer.
- `Zoek de offerte van Carola Dekker` -> resolve customer and retrieve matching quote data.
- `Die van Carola Dekker` after that -> continue the pending quote lookup for Carola Dekker.
- `Ja` after `Ik heb Carola Dekker gevonden` -> fetch the requested details directly if the earlier task already established which detail is wanted.
- `Carola Dekker` two days later after an old unresolved lookup -> usually treat as a fresh intent unless the message explicitly refers back to the earlier question.
- `Morgen` after discussing appointments -> treat as narrowing the active appointment question to tomorrow.
- `Doe maar` after offering to send a quote PDF -> continue with the pending send action if the target quote is already clear.