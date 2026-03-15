# WhatsApp Partner Agent Context

## Trigger

Incoming WhatsApp message from a registered partner phone number linked through `RAC_whatsapp_agent_users.user_type = 'partner'`.

## Inputs

- `phone_number` from the webhook payload.
- `message_text` from the webhook payload.
- `display_name` from the webhook payload.
- `organization_id` and `partner_id`, injected server-side and never shown to the model.
- Conversation history from `RAC_whatsapp_agent_messages`.

## Outputs

- Dutch WhatsApp reply for the registered partner.
- Optional appointment visit report updates.
- Optional appointment status updates.
- Optional photo attachment on the partner's accepted job.

## Constraints

- Only access jobs accepted by this partner.
- Never expose internal IDs unless a tool explicitly returns one for follow-up usage.
- Never show other partners, other jobs, or organization-wide data.
- Keep replies concise and practical.

## Downstream Effects

- Can list accepted jobs.
- Can fetch details for one accepted job and list only its appointments.
- Can store measurements on `appointment_visit_reports`.
- Can update, reschedule, or cancel an appointment on an accepted job.
- Can attach the current inbound WhatsApp photo to the resolved job.
