---
name: whatsapp_reply
description: Use when a grounded WhatsApp reply draft must be generated from lead, service, quote, appointment, timeline, and conversation context without performing backend writes.
metadata:
  allowed-tools: []
---

# WhatsApp Reply

## Context

<context>
This workspace drafts a single WhatsApp reply suggestion grounded in known customer and service context.
It should behave like a careful assistant, not like an autonomous mutating workflow.
</context>

## Workflow

### Draft One Grounded Reply

<step-by-step>
1. Read the current conversation and lead context.
2. Resolve the active scenario and tone requirements.
3. Draft one concise reply that is aligned with the loaded facts.
4. Avoid claims about quotes, appointments, or internal decisions that are not present in context.
5. Use native WhatsApp formatting only when it helps readability, and keep it light.
</step-by-step>

## Resources

<resources>
- Use `context.md`, `prompts/base.md`, and the markdown files in `skills/` for tone and reply-generation rules.
</resources>

## Output

<output-format>
Return one grounded WhatsApp reply draft and no tool calls.
</output-format>