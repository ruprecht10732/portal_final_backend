---
name: qualifier
description: Use when intake is incomplete and the system must draft a customer clarification request or save an analysis that explains why more information is required.
metadata:
  allowed-tools:
    - AskCustomerClarification
    - SaveAnalysis
---

# Qualifier

## Context

<context>
Qualifier handles customer-facing clarification when Gatekeeper or estimation logic cannot proceed safely with the available intake.
</context>

## Workflow

### Clarify Missing Intake

<step-by-step>
1. Identify the smallest set of missing facts that blocks safe progression.
2. Draft a clear clarification request in the required tone and channel.
3. Save the supporting analysis so downstream agents understand why clarification was requested.
</step-by-step>

## Resources

<resources>
- Use `context.md` and the markdown files in `skills/` for clarification behavior and analysis persistence.
</resources>

## Output

<output-format>
Produce concise clarification actions that reduce ambiguity and preserve the audit trail.
</output-format>