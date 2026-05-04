# Agent Workspace Architecture

This repository uses a convention-based file workspace model for backend agents.

## Goals

- Keep static agent behavior on disk in markdown workspaces.
- Keep runtime data composition, typed tool contracts, invariants, and orchestrator logic in Go.
- Fail fast at startup when the workspace is incomplete or invalid.
- Make loaded workspace versions observable in logs.

## Package Roles

- `internal/orchestration/`: workspace loading, parsing, instruction composition, and ADK toolset filtering.
- `internal/tools/`: central tool catalog and the shared boundary for system and domain tool naming.
- `internal/leads/agent/`: lead-domain agent runtimes, prompt assembly, and domain-specific tool implementations.

## Unified Agent Runtime (v2.0)

`internal/leads/agent/` is anchored by a single `Runtime` struct that dynamically constructs workspace-specific agents on demand. This eliminates the need for the `leads.Module` to hold separate fields for each agent.

- `Runtime` receives an `AgentTaskPayload` specifying the target `Workspace` (`gatekeeper`, `calculator`, `matchmaker`, `auditor`) and optional `Mode`.
- Shared dependencies (repository, event bus, model configs, session service, catalog reader, quote drafter, partner offer creator) are injected once at module initialization.
- The scheduler consumes a single `AgentTaskScheduler` interface (`EnqueueAgentTask`) and dispatches the unified `agent:run` task type.
- `leads.Module` implements `scheduler.LeadAutomationProcessor.ProcessAgentTask` to bridge scheduler tasks back into `Runtime.Run`.

## Layout

- `AGENTS.md`: repository root identity.
- `agents/README.md`: master navigation entry point for the workspace.
- `agents/shared/`: shared identity, governance, and prompt fragments.
- `agents/shared/glossary.md`: canonical terminology.
- `agents/shared/tool-reference.md`: deeper tool contracts.
- `agents/shared/integration-guide.md`: support-agent choreography.
- `agents/shared/error-handling.md`: failure and recovery playbook.
- `agents/<role>/context.md`: role-level behavior.
- `agents/<role>/INTEGRATION.md`: lifecycle trigger, input, output, and downstream contract for primary roles.
- `agents/<role>/prompts/*.md`: static prompt sources for larger templates.
- `agents/<role>/skills/*.md`: tool-facing skill guidance.
- `SKILL.md` frontmatter: workspace identity, trigger description, and allowed-tools boundary.

## Runtime Composition

The loader in `internal/orchestration/workspace.go` composes instructions in this order:

1. `AGENTS.md`
2. shared `SKILL.md`
3. shared markdown resources discovered by convention from the workspace folders
4. agent `SKILL.md`
5. agent markdown resources discovered by convention from `context.md`, root markdown files, `prompts/`, `skills/`, and `references/`
6. generated workspace persistence section

`BuildAgentInstruction(agentName, extraSections...)` can append runtime-specific sections after the workspace content when a prompt still has dynamic addenda such as tenant tone.

## Validation

Startup now performs two validation passes in `internal/leads/module.go` before agents are constructed:

1. `ValidateAgentWorkspaces()` checks that the required workspace entrypoints and convention-based markdown resources resolve correctly.
2. `ValidatePromptTemplates()` executes the parsed Go templates with synthetic data to catch placeholder mismatches early.

This keeps invalid template fields or broken file references from surfacing only during the first live request.

## Observability

When a workspace is loaded, the loader logs:

- agent name
- workspace root
- short content hash of the composed instruction
- loaded file list

This makes it possible to correlate runtime behavior with a specific workspace composition during debugging or incident review.

## Design Boundary

Markdown is the source of truth for:

- agent identity
- static prompt rules
- workflow guidance
- skill descriptions

Go remains the source of truth for:

- tool schemas and implementations
- database writes
- orchestrator behavior
- state reconciliation
- pipeline and status invariants
- dynamic lead, service, quote, photo, and tenant context

If markdown and Go disagree, Go wins.

## Maintenance Rules

- Prefer editing markdown when changing static behavior or wording.
- Prefer editing Go when changing business rules, validators, tool behavior, or dynamic context assembly.
- When adding a new agent workspace, add its files first, then register its directory in `internal/orchestration/workspace.go`, then add startup validation coverage.
- When adding new template placeholders, update the corresponding validation data in `prompt_template_validation.go`.
- Keep `SKILL.md` frontmatter and workspace markdown aligned with the actual runtime trigger and tool boundaries.