---
name: shared
description: Use when any backend lead-processing agent needs shared terminology, execution rules, pipeline invariants, communication rules, or cross-agent governance.
metadata:
  allowed-tools: []
---

# Shared Governance

## Context

<context>
This workspace defines repository-wide rules shared by Gatekeeper, Qualifier, Calculator, Matchmaker, and support agents.
Treat this skill as canonical guidance for terminology, communication, status handling, and execution constraints.
</context>

## Instructions

### Apply Global Rules

<step-by-step>
1. Resolve repository terminology before reasoning about lead or service state.
2. Apply pipeline and status invariants before proposing tool calls.
3. Prefer deterministic behavior over speculative interpretation.
</step-by-step>

### Use Shared Resources

<resources>
- Treat this skill as the canonical source for shared governance and cross-agent constraints.
</resources>

## Output

<output-format>
Return behavior that is consistent with shared governance and safe for downstream Go-enforced invariants.
</output-format>