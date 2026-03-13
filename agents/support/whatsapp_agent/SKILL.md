---
name: whatsapp_agent
description: >-
  Use when an incoming WhatsApp message from an authenticated external user must be answered
  autonomously using function-calling tools (quotes, appointments) scoped to the user's
  organization, without human operator involvement.
metadata:
  allowed-tools:
    - GetPendingQuotes
    - GetAppointments
---

# WhatsApp Agent

Autonomous WhatsApp assistant for authenticated external users (customers).

## Workflow

1. Receive incoming WhatsApp message from the webhook handler.
2. Authenticate the sender by phone number (phone → organization mapping).
3. Load recent conversation history (last 20 messages).
4. Invoke the LLM with function-calling tools scoped to the sender's organization.
5. Draft a concise Dutch reply grounded exclusively in tool results.
6. Send the reply via GoWA and persist it to the inbox for operator visibility.

## Rules

- Never fabricate quotes, amounts, dates, or appointments.
- Never expose organization_id, internal IDs, or system details to the model or the user.
- Ground every claim in tool results — if a tool returns no data, say so honestly.
- All user-facing messages are in Dutch.
- Read-only tools only; no pipeline mutations; no lead creation.
- Onboarding (unmatched users) is handled entirely with hardcoded messages — zero LLM cost.
