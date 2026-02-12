# Workflow Resolver Behavior (M2)

## Purpose
This document defines how the backend resolves the effective workflow for a lead.

## Entry point
- Service method: `ResolveLeadWorkflow`
- File: `internal/identity/service/service.go`

Input model:
- `organizationId` (required)
- `leadId` (required)
- `leadSource` (optional)
- `leadServiceType` (optional)
- `pipelineStage` (optional)

Output model:
- `workflow` (optional)
- `resolutionSource`
- `overrideMode` (optional)
- `matchedRuleId` (optional)

## Resolution precedence
The resolver applies this strict order:

1. Manual override
2. Automatic assignment rules
3. Organization default behavior

### 1) Manual override
The resolver checks `RAC_lead_workflow_overrides` for `(lead_id, organization_id)`.

Outcomes:
- If `override_mode = clear`:
  - `resolutionSource = manual_clear`
  - No workflow selected.
- If `override_mode != clear` and `workflow_id` exists and points to an enabled workflow:
  - `resolutionSource = manual_override`
  - The referenced workflow is selected.
- If override exists but references a missing/disabled workflow:
  - `resolutionSource = manual_override`
  - No workflow selected (safe fallback behavior).

### 2) Automatic assignment rules
If no manual override is applied, rules are read from `RAC_workflow_assignment_rules`
ordered by `priority ASC, created_at ASC`.

Rule matching behavior:
- Disabled rules are skipped.
- A rule field (`lead_source`, `lead_service_type`, `pipeline_stage`) matches when:
  - rule field is null/empty (wildcard), or
  - rule field equals the input value case-insensitively.
- First matching rule wins.
- The matched rule selects its target workflow only if that workflow is enabled.

Outcome:
- `resolutionSource = auto_rule`
- `matchedRuleId` set to selected rule id.

### 3) Organization default
If no manual override and no matching auto-rule lead to an enabled workflow:
- `resolutionSource = organization_default`
- No workflow selected by resolver.

Downstream services must then apply existing organization defaults.

## Enabled workflow filter
Before applying override/rule selection, workflows are filtered to enabled workflows only.
Disabled workflows are never selected by the resolver.

## Error handling
The resolver returns errors for data-access failures, except:
- `lead override not found` is treated as a normal flow state.

## Determinism guarantees
For a fixed input snapshot:
- Resolver output is deterministic.
- Rule tie-break is stable due to `priority ASC, created_at ASC` ordering.

## Current test coverage
Unit tests in `internal/identity/service/workflow_resolver_test.go` validate:
- Optional field matching behavior.
- Assignment-rule case-insensitive matching.
- Manual override handling (`clear`, valid workflow, missing workflow).

## Notes for M3/M4 integration
- M3 runtime must consume `resolutionSource` to support observability.
- M4 quote term overrides should read selected workflow overrides when `workflow != nil`,
  otherwise fallback to organization settings.
