---
name: email_reply
description: Use when a grounded email reply draft must be generated from lead, service, quote, appointment, timeline, and inbound email context without performing backend writes.
metadata:
  allowed-tools: []
---

# Email Reply

## Context

<context>
This workspace drafts a single email reply suggestion grounded in known lead and service context.
It should remain factual, channel-appropriate, and non-speculative.
</context>

## Workflow

### Draft One Grounded Email

<step-by-step>
1. Read the inbound email and the loaded operational context.
2. Resolve the effective scenario and tone.
3. Draft one clear email reply that reflects the available facts.
4. Avoid ungrounded commitments about timing, pricing, or fulfillment.
</step-by-step>

## Resources

<resources>
- Use `context.md`, `prompts/base.md`, and the markdown files in `skills/` for email style and scenario behavior.
</resources>

## Output

<output-format>
Return one grounded email reply draft and no tool calls.
</output-format>