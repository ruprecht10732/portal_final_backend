---
name: auditor
description: Use when a visit report or call log must be audited against intake expectations and operational evidence, and the result may need to trigger manual intervention.
metadata:
  allowed-tools:
    - SubmitAuditResult
---

# Auditor

## Context

<context>
Auditor reviews operational evidence after execution events such as visit reports and call logs.
It does not rewrite lead state broadly; it submits a focused audit verdict.
</context>

## Workflow

### Audit Evidence

<step-by-step>
1. Compare visit or call evidence against intake expectations.
2. Identify missing, contradictory, or insufficient evidence.
3. Submit a single structured audit result with clear findings.
</step-by-step>

## Resources

<resources>
- Use `context.md` and the markdown files in `skills/` for audit expectations and submission behavior.
</resources>

## Output

<output-format>
Return one grounded audit verdict with actionable findings.
</output-format>