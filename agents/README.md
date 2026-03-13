# Agent Workspace Index

This directory is the file-based source of truth for backend agent behavior.

## Purpose

- Give humans and AI systems one navigable entry point into the lead automation stack.
- Keep static behavior, trigger descriptions, and tool guidance in markdown.
- Keep runtime data loading in `internal/orchestration/`, shared tool naming in `internal/tools/`, and domain execution logic in Go.

## Primary Agents

- `gatekeeper/`: intake validation and readiness decisions.
- `qualifier/`: clarification messaging when intake is incomplete.
- `calculator/`: scoping, pricing, quote drafting, quote critique, and quote repair.
- `matchmaker/`: partner matching and fulfillment routing.

## Support Agents

- `support/photo_analyzer/`: visual evidence extraction and discrepancy detection.
- `support/call_logger/`: post-call note and appointment normalization.
- `support/auditor/`: audits for visit reports and call logs.
- `support/offer_summary/`: concise partner-offer summaries.
- `support/whatsapp_reply/`: grounded WhatsApp reply suggestions.
- `support/email_reply/`: grounded email reply suggestions.
- `support/whatsapp_agent/`: autonomous WhatsApp AI assistant for authenticated external users.

## Trigger Graph

1. Lead creation or service creation can trigger Gatekeeper.
2. Image attachments defer Gatekeeper until PhotoAnalyzer completes or fails.
3. Gatekeeper can keep a service in `Nurturing`, move it to `Estimation`, or escalate to `Manual_Intervention`.
4. `Estimation` can trigger Calculator-runtime flows.
5. `Fulfillment` can trigger Matchmaker/Dispatcher.
6. Visit reports and call logs can trigger Auditor.
7. Communication surfaces can request reply agents and offer summaries on demand.

## Reading Order

1. `../AGENTS.md`
2. `shared/glossary.md`
3. `shared/tool-reference.md`
4. `../docs/agent-workspace-architecture.md`
5. `../docs/agent-runtime-flow.md`
6. Agent-specific `context.md` and `INTEGRATION.md`

## Maintenance Checklist

When adding or changing an agent workspace:

1. Keep `SKILL.md` frontmatter accurate for `name`, `description`, and `allowed-tools`.
2. Keep `context.md` self-contained: trigger, inputs, outputs, dependencies, fallback.
3. Store larger static prompt rules under `prompts/`.
4. Keep tool-facing operational instructions under `skills/`.
5. Update `shared/glossary.md` if you introduce new workflow terms.
6. Update `shared/tool-reference.md` if tool contracts change.
7. Update `../docs/agent-runtime-flow.md` if the trigger graph changes.
8. Register new workspace directories in `internal/orchestration/workspace.go` and keep `allowed-tools` aligned with `internal/tools/domain_tools.go`.