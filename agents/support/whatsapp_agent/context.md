# WhatsApp Agent Context

## Trigger

Incoming WhatsApp message from an authenticated (phone-linked) external user, dispatched by the webhook handler after inbox persistence.

## Inputs

- `phone_number` from the webhook payload.
- `message_text` from the webhook payload.
- `display_name` from the webhook payload.
- `conversation_history` from `RAC_whatsapp_agent_messages`, including recency information for continuity decisions.
- `organization_id` from `RAC_whatsapp_agent_users`, injected server-side and never shown in the prompt.

## Outputs

- Natural language Dutch reply sent via GoWA.
- Reply persisted to `RAC_whatsapp_agent_messages` for history.
- Reply written to the inbox for operator visibility.

## Constraints

- organization_id is never visible to the LLM — injected server-side into tool handlers.
- Maximum 10 function-calling iterations per request.
- The runtime provides recent conversation context, but older turns should be treated cautiously so stale intents do not leak into a new request.
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
- Tool technical failure → operator logs should contain the failure details; the customer should receive a short, calm retry-later message instead of raw errors.
- The Go layer should prefer model autonomy over deterministic pre-routing, except for hard safety boundaries like auth, rate limiting, and tenant scoping.

## Autonomy Notes

- Broad overview questions like `Welke afspraken zijn er?` or `Welke offertes zijn er?` should trigger the appropriate listing tool and a direct summary of the results.
- A follow-up like `Die van Carola Dekker` after a quote request should be treated as disambiguation of the pending quote search, not as a brand-new vague request.
- Once the target customer is resolved and exactly one quote or one relevant result remains, answer directly instead of asking another clarification question.
