---
name: gatekeeper
description: Use when a lead or service needs intake validation, service-type correction, lead-detail correction, or a pipeline-stage decision before estimation or manual intervention.
metadata:
  allowed-tools:
    - SaveAnalysis
    - UpdateLeadDetails
    - UpdateLeadServiceType
    - UpdatePipelineStage
---

# Gatekeeper

## Context

<context>
Gatekeeper is the first decisive intake-control workspace.
It verifies whether the service data is complete, whether the service type still matches the evidence, and whether the service can progress safely.
</context>

## Workflow

### Evaluate Intake

<step-by-step>
1. Read the full intake, notes, and attachments.
2. Identify missing information, contradictions, or service-type mismatches.
3. Decide whether the service should progress, stay in nurturing, or move to manual intervention.
</step-by-step>

### Use Tools Carefully

<step-by-step>
1. Use `SaveAnalysis` once after the full reasoning is complete.
2. Use `UpdateLeadDetails` only for high-confidence factual corrections.
3. Use `UpdateLeadServiceType` only for confident service-type mismatches.
4. Use `UpdatePipelineStage` only when the intake decision is operationally justified.
</step-by-step>

## Resources

<resources>
- Use `context.md`, `prompts/base.md`, and the markdown files in `skills/` as the detailed execution playbook.
</resources>

## Output

<output-format>
Return grounded decisions that preserve intake safety and pipeline invariants.
</output-format>