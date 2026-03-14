# WhatsApp Agent Context

## Trigger

Incoming WhatsApp message from an authenticated (phone-linked) external user, dispatched by the webhook handler after inbox persistence.

## Inputs

| Input              | Source               | Notes                                         |
|--------------------|----------------------|-----------------------------------------------|
| phone_number       | webhook payload      | Sender's WhatsApp number                      |
| message_text       | webhook payload      | Raw message body                              |
| display_name       | webhook payload      | Sender's WhatsApp profile name                |
| conversation_history | RAC_whatsapp_agent_messages | Last 100 messages for context continuity |
| organization_id    | RAC_whatsapp_agent_users | Injected server-side, never in prompt       |

## Outputs

- Natural language Dutch reply sent via GoWA.
- Reply persisted to `RAC_whatsapp_agent_messages` for history.
- Reply written to the inbox for operator visibility.

## Constraints

- organization_id is never visible to the LLM — injected server-side into tool handlers.
- Maximum 10 function-calling iterations per request.
- The runtime provides a deep recent conversation window so the model can continue multi-turn chats naturally instead of restarting after a few turns.
- Rate limited: 30 messages per 5 minutes per phone number.
- Do not set status to Disqualified via this agent.

## Downstream Effects

- Operator sees AI replies in the WhatsApp inbox (read-only visibility).
- Agent can create leads, update lead details, save notes, ask clarifications, and manage appointments.
- Agent cannot mutate pipeline stages or set status to Disqualified.

## Failure Modes

- Phone not matched → hardcoded onboarding flow (zero LLM cost).
- Rate limit exceeded → hardcoded rate-limit message.
- LLM error → logged; no reply sent (fail silent).
- Tool returns no data → agent responds honestly ("no results found").
- The Go layer should prefer model autonomy over deterministic pre-routing, except for hard safety boundaries like auth, rate limiting, and tenant scoping.
