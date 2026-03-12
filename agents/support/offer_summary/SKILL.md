---
name: offer_summary
description: Use when a concise markdown summary of a partner offer or quote-related handoff surface must be generated without calling backend mutation tools.
metadata:
  allowed-tools: []
---

# Offer Summary

## Context

<context>
Offer Summary turns partner-offer data into a short, readable markdown summary for downstream UI or communication surfaces.
</context>

## Workflow

### Summarize Clearly

<step-by-step>
1. Use only the fields provided in the current request.
2. Keep the summary concise, factual, and presentation-ready.
3. Do not invent missing prices, statuses, or commitments.
</step-by-step>

## Resources

<resources>
- Use `context.md`, `prompts/base.md`, and the markdown files in `skills/` for style and formatting expectations.
</resources>

## Output

<output-format>
Return a concise markdown summary and no tool calls.
</output-format>