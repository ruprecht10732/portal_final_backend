# WhatsApp Agent Context

## Trigger

Incoming WhatsApp message from an authenticated (phone-linked) external user, dispatched by the webhook handler after inbox persistence.

## Inputs

| Input              | Source               | Notes                                         |
|--------------------|----------------------|-----------------------------------------------|
| phone_number       | webhook payload      | Sender's WhatsApp number                      |
| message_text       | webhook payload      | Raw message body                              |
| display_name       | webhook payload      | Sender's WhatsApp profile name                |
| conversation_history | RAC_whatsapp_agent_messages | Last 20 messages for context continuity |
| organization_id    | RAC_whatsapp_agent_users | Injected server-side, never in prompt       |

## Outputs

- Natural language Dutch reply sent via GoWA.
- Reply persisted to `RAC_whatsapp_agent_messages` for history.
- Reply written to the inbox for operator visibility.

## Constraints

- Read-only tools only: no pipeline mutations, no lead creation.
- organization_id is never visible to the LLM — injected server-side into tool handlers.
- Maximum 10 function-calling iterations per request.
- Rate limited: 30 messages per 5 minutes per phone number.

## Downstream Effects

- Operator sees AI replies in the WhatsApp inbox (read-only visibility).
- No lead or service state changes are made by this agent.

## Failure Modes

- Phone not matched → hardcoded onboarding flow (zero LLM cost).
- Rate limit exceeded → hardcoded rate-limit message.
- LLM error → logged; no reply sent (fail silent).
- Tool returns no data → agent responds honestly ("no results found").
