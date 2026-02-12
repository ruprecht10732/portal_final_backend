# Workflow Engine — Migration & Rollout Guide (M5)

## Objective
Roll out workflow-engine backend changes safely while preserving existing behavior for active tenants.

## Change inventory
- Migration `090_workflow_engine_foundation.sql` (additive schema only).
- Runtime workflow step executor integration in notification module.
- Quote communication trigger integration (`quote_sent`, `quote_accepted`, `quote_rejected`) with fallback/suppression semantics.
- Quote event payload enrichment for accepted/rejected paths.

## Pre-deploy checklist
1. Ensure branch includes:
   - migration `090_workflow_engine_foundation.sql`,
   - M2/M3/M4 backend code changes,
   - updated docs.
2. Confirm environment has required services:
   - PostgreSQL,
   - Redis (scheduler),
   - email/whatsapp provider configuration as applicable.
3. Confirm quality gates in target commit:
   - `go build ./...`
   - `go test ./...`
   - `golangci-lint run ./...`

## Deployment order
1. Deploy backend with migration runner enabled.
2. Let startup apply pending migrations automatically.
3. Start/verify API nodes.
4. Start/verify scheduler workers that process notification outbox records.

Rationale:
- schema is additive and safe to apply before/with code,
- runtime depends on outbox scheduler for delayed workflow delivery.

## Post-deploy validation
Run smoke tests in this order:
1. Admin workflow settings:
   - `GET /api/v1/admin/organizations/me/workflow-engine/workflows`
   - `PUT /api/v1/admin/organizations/me/workflow-engine/workflows`
2. Quote sent path:
   - call `POST /api/v1/quotes/:id/send`,
   - verify notification side effects follow rule/no-rule/disabled behavior.
3. Public quote acceptance path:
   - call `POST /api/v1/public/quotes/:token/accept`,
   - verify quote accepted communications and activity updates.
4. Public quote rejection path:
   - call `POST /api/v1/public/quotes/:token/reject`,
   - verify workflow-driven or no-rule behavior (no direct outbound).
5. Outbox health:
   - confirm pending jobs are claimed and moved to succeeded/failed with retry policy.

## Rollback strategy
### Code rollback
- Roll back application binaries/images to previous stable version.

### Data rollback
- Prefer forward-fix over destructive rollback.
- `090` is additive; keeping tables in place is generally safer than dropping under pressure.
- Only run migration down scripts in controlled maintenance windows with explicit approval.

### Behavior rollback
If runtime behavior is problematic after deploy:
- disable problematic workflow rules via `/admin/organizations/me/workflow-engine/workflows` to suppress specific channels,
- rely on legacy fallback paths where implemented,
- keep audit trail in logs while investigating.

## Operational guardrails
- Monitor notification retry logs and failed outbox records after rollout.
- Watch quote action success rates (`send`, `accept`, `reject`) for regressions.
- Keep fallback semantics documented and unchanged until explicit product sign-off for stricter workflow-only behavior.

## Release note template
- Added additive workflow-engine schema foundation (`090`).
- Added workflow-aware quote communication routing for sent/accepted/rejected triggers.
- Preserved legacy behavior fallback for non-configured paths where legacy existed.
- No frontend-breaking API changes for existing workflow settings endpoints.
