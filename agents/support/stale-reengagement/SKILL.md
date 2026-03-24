---
name: stale-reengagement
description: Analyze a stale lead service and generate a re-engagement recommendation with a draft contact message.
metadata:
  allowed-tools: []
---

# Stale Lead Re-Engagement

## Context

<context>
This workspace generates a proactive re-engagement suggestion for a stale lead service.
It analyzes the lead's history, current pipeline status, stale reason, and available context
to recommend a concrete follow-up action with a ready-to-send draft message.
</context>

## Workflow

### Generate Re-Engagement Suggestion

<step-by-step>
1. Read the stale reason and lead context (timeline, notes, analysis, stage, service type).
2. Determine the most effective re-engagement action based on the stale reason and history.
3. Choose the preferred contact channel (whatsapp or email) based on available contact info and prior communication.
4. Draft a short, warm, actionable contact message in Dutch that the operator can send directly.
5. Write a brief internal summary explaining the recommendation rationale.
6. Return a structured JSON response with: recommended_action, suggested_contact_message, preferred_contact_channel, summary.
</step-by-step>

## Resources
