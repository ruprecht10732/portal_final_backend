# Backend Agents

This repository uses a folder-as-a-workspace model for backend lead-processing agents.

## Canonical Terms

- Lead: a customer record that can contain one or more services.
- Service: a single work request inside a lead and the main unit for pipeline progression.
- Gatekeeper: the intake validation agent that decides whether a service is ready to continue.
- Qualifier: the clarification agent that drafts customer follow-up when intake is incomplete.
- Calculator: the estimation and quote-generation workspace used by the estimator and quote generator.
- Matchmaker: the fulfillment-routing workspace used by the dispatcher.
- Otera: partner-facing fulfillment and offer-routing context in this codebase.

## Workspace Contract

- Each workspace is anchored by a `SKILL.md` file with YAML frontmatter.
- The `description` field is the trigger text for agent selection and must describe what the workspace does and when it should be used.
- The `allowed-tools` field is the safety boundary for tool exposure. Go code may construct more tools, but only the allowed subset is exposed to the model.
- Additional markdown in `context.md`, `prompts/`, `skills/`, and `references/` is treated as workspace resources and loaded after the entry `SKILL.md`.

## Global Rules

- Never invent missing customer, pricing, or scheduling facts.
- Prefer durable tool calls for state changes; do not simulate database writes in plain text.
- Respect lead, service, quote, appointment, and pipeline invariants enforced in Go.
- When markdown and Go disagree, Go is authoritative for runtime behavior.
- Keep outputs grounded in tenant, lead, and service context already loaded by the caller.

## Entry Point

- `AGENTS.md` is the only supported root workspace entrypoint.