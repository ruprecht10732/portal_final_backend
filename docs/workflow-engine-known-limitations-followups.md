# Workflow Engine — Known Limitations & Follow-ups (M5)

## Known limitations (current state)
1. Workflow-engine foundation is not HTTP-exposed yet.
   - Tables and service logic exist, but no admin/public routes for workflow CRUD, assignment rules, lead overrides, or resolver preview.
2. Workflow execution still depends on lead-context resolution quality.
   - Missing lead context can prevent assignment-rule matches.
3. Quote rejection path has no legacy outbound fallback by design.
   - If no workflow rule exists, behavior remains no direct outbound communication.
4. Runtime channel delivery confirmation is asynchronous.
   - API success for quote actions does not imply immediate message delivery.
5. Observability is log/outbox-centric.
   - No dedicated admin dashboard yet for workflow execution/retry/failure analytics.

## Risks to track
- Contract confusion between legacy workflow settings and new foundation model.
- Tenant misconfiguration causing unintended suppression when rules are disabled.
- Operational noise from transient provider failures (email/whatsapp) increasing retry volume.

## Follow-up backlog (recommended order)
1. Expose workflow-engine HTTP APIs
   - Workflows + steps CRUD
   - Assignment rules CRUD
   - Lead override CRUD
   - Resolver preview endpoint for debugging effective workflow selection
2. Add integration tests for quote rejected workflow send path
   - Include enabled/disabled/no-rule matrix validation
3. Add execution observability surfaces
   - Admin endpoint or dashboard for outbox retries/failures by trigger/channel
4. Add migration path for frontend from legacy rule editor to full workflow-engine editor
5. Add explicit contract versioning doc once workflow-engine HTTP endpoints are published

## Exit criteria for closing these follow-ups
- Frontend uses workflow-engine HTTP contracts instead of legacy-only settings API.
- Effective workflow resolution is inspectable per lead via API.
- Delivery failures and retry trends are observable without log scraping.
- Product signs off on final fallback/suppression behavior for all quote trigger variants.
