# Error Handling Guide

## General Rule

If markdown guidance and runtime behavior appear to disagree during an error path, trust the Go runtime invariants and repository validation.

## Workspace Validation Failures

- Startup must fail when required workspace entry files or prompt templates are invalid.
- Do not silently ignore missing `SKILL.md`, `AGENTS.md`, or required workspace markdown files.

## Queue Or Scheduler Unavailable

- Prefer explicit warnings and fail-fast logs over silent degradation.
- If a workflow falls back from async to sync execution, log that fact clearly.

## Photo Analysis Failure

- Preserve an audit trail with a timeline alert.
- Allow Gatekeeper to continue with reduced evidence when backend policy says that is useful.

## Call Logger Dependency Gaps

- Missing appointment booker or lead updater should be visible in logs and setup validation.
- Do not pretend a booking or update succeeded when a dependency is missing.

## Reply Agent Context Gaps

- Missing quote or appointment readers should degrade gracefully but remain visible in diagnostics.
- Never fabricate the missing context in reply drafts.

## Manual Intervention

- Use `Manual_Intervention` when automation cannot proceed safely.
- Include reason codes and enough evidence for a human to continue without re-running blind.