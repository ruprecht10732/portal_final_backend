# Workflow Engine — Backend Implementation Plan

## Context
This feature introduces a full workflow engine for notifications and commercial defaults, with:
- lead-level workflow assignment (automatic + manual override),
- multi-channel messaging (WhatsApp + Email),
- configurable recipients,
- unlimited follow-up steps,
- quote-term overrides from workflow (payment term + validity term),
- backend-first delivery before frontend handover.

---

## Working Agreement

### Branching
- Backend branch: `feature/workflow-engine-backend`
- No direct commits to default branch for this feature.

### Backend-first policy
- We complete backend implementation and documentation first.
- Frontend handover starts only after all backend DoD gates are green.

### Tracking discipline
- This file is the single source of truth for progress.
- Every completed item must be checked in this file in the same PR as code changes.

---

## Definition of Done (DoD)
A backend work item is **Done** only if all conditions below are met:

1. **Functional completeness**
   - Behavior matches acceptance criteria.
   - Backward compatibility is preserved or explicitly migrated.

2. **Code quality**
   - Build succeeds.
   - Linting succeeds.
   - Relevant tests pass.

3. **Documentation completeness**
   - API and behavior changes documented.
   - Migration notes and rollout notes documented.

4. **Operational safety**
   - Logging for critical workflow execution paths present.
   - Failure/fallback behavior documented and tested.

5. **Handover readiness**
   - Backend contract stable enough for frontend integration.
   - Frontend handover document updated.

### Required quality commands (must run before marking Done)
> Run in `portal_final_backend`:

- Build:
  - `go build ./...`
- Test:
  - `go test ./...`
- Lint (if golangci-lint configured):
  - `golangci-lint run`

If any command is unavailable in environment, record it explicitly under the item notes with reason and alternative verification.

---

## Scope (Backend)

### 1) Data model and migrations
- Add workflow model for:
  - workflow definitions,
  - step sequences (unlimited),
  - recipient targeting,
  - assignment rules,
  - lead manual override,
  - quote-term override fields.
- Preserve compatibility with existing `RAC_notification_workflows` records.

### 2) Workflow assignment engine
- Automatic matching to lead context.
- Manual override support at lead level.
- Deterministic precedence: manual override > automatic match > organization defaults.

### 3) Notification orchestration engine
- Replace hardcoded dispatch paths with workflow-driven execution where in scope.
- Support channel-specific rendering and send routing.
- Support delayed and chained follow-up steps.

### 4) Quote terms override integration
- If lead has active workflow with quote overrides:
  - use workflow payment term,
  - use workflow validity term.
- Else fallback to organization settings.

### 5) API/contracts for frontend
- Stable DTOs for:
  - workflow CRUD,
  - step CRUD,
  - recipient config,
  - assignment config,
  - lead override,
  - effective workflow resolution (debug/preview endpoint if needed).

### 6) Observability and failure handling
- Structured logs for workflow selection, step execution, retries/failures.
- Explicit fallback rules for missing/invalid config.

---

## Milestones and Checkpoints

## M1 — Foundation (Schema + Contracts)
- [x] Design DB schema changes
- [x] Create migrations
- [x] Repository layer for new workflow entities
- [x] Transport DTO updates
- [x] Validation rules
- [x] Compatibility mapping from old workflow table

**Checkpoint M1 DoD**
- [x] `go build ./...`
- [x] `go test ./...`
- [x] `golangci-lint run`
- [x] Migration notes documented

## M2 — Assignment and Resolution
- [x] Implement automatic assignment rules
- [x] Implement manual lead override
- [x] Implement precedence resolver
- [x] Add tests for resolver edge-cases

**Checkpoint M2 DoD**
- [x] `go build ./...`
- [x] `go test ./...`
- [x] `golangci-lint run`
- [x] Resolver behavior documented

## M3 — Orchestration Runtime
- [x] Implement step executor (unlimited sequence)
- [x] Implement per-step channel routing (WhatsApp/Email)
- [x] Implement recipient matrix resolution
- [x] Integrate with outbox/scheduler path
- [x] Add retry/failure handling policy

**Checkpoint M3 DoD**
- [x] `go build ./...`
- [x] `go test ./...`
- [x] `golangci-lint run`
- [x] Runtime sequence behavior documented

## M4 — Quote Override + Existing Flow Integration
- [x] Apply workflow payment/validity overrides in quote generation path
- [x] Integrate accepted/rejected/sent communication paths
- [x] Keep fallback behavior for non-workflow leads
- [x] Add integration tests for quote + notification behavior

**Checkpoint M4 DoD**
- [x] `go build ./...`
- [x] `go test ./...`
- [x] `golangci-lint run`
- [x] API/behavior docs updated

## M5 — Backend Handover Pack
- [x] Final backend API contract document
- [x] Frontend integration guide
- [x] Migration/rollout guide
- [x] Known limitations and follow-ups list

**Checkpoint M5 DoD**
- [x] `go build ./...`
- [x] `go test ./...`
- [x] `golangci-lint run`
- [x] Handover approved internally

---

## Handover Gate to Frontend
Frontend handover may start only when all conditions are true:
- [x] M1–M5 complete
- [x] Backend quality commands green on latest branch
- [x] API contracts frozen for first frontend iteration
- [x] Required example payloads added to docs
- [x] Breaking changes and migration strategy explicitly documented

---

## Progress Log
Use this section to track what was implemented in each update.

### 2026-02-12
- Created feature branch `feature/workflow-engine-backend`.
- Created this plan document with DoD and milestone tracking.
- Added migration `090_workflow_engine_foundation.sql` with additive workflow-engine tables (`RAC_workflows`, `RAC_workflow_steps`, `RAC_workflow_assignment_rules`, `RAC_lead_workflow_overrides`).
- Added compatibility helper view `RAC_workflow_legacy_notification_rules` for legacy mapping visibility.
- Added workflow-engine foundation DTO contracts in `internal/identity/transport/dto.go` for workflows, steps, assignment rules, and lead overrides.
- Verification snapshot (current branch state):
  - `go build ./...` ✅
  - `go test ./...` ✅
  - `golangci-lint run` ✅ (0 issues)
- Created sub-branch `feature/workflow-engine-backend-m1-repository-validation` for isolated M1 repository/validation work.
- Added repository foundation file `internal/identity/repository/workflow_engine.go` with CRUD/replace methods for workflows, steps, assignment rules, and lead overrides.
- Tightened validation tags for workflow-engine DTO contracts in `internal/identity/transport/dto.go` (channel/action/audience enums, min-length constraints, non-empty collections).
- Re-ran quality gates after repository + validation work:
  - `go build ./...` ✅
  - `go test ./...` ✅
  - `golangci-lint run` ✅ (0 issues)
- Documented migration notes for `090_workflow_engine_foundation.sql` in `docs/MIGRATIONS.md`.
- Started M2 assignment/resolution implementation in `internal/identity/service/service.go`:
  - added workflow CRUD service pass-through for new engine repository methods,
  - added lead override upsert/delete service methods,
  - added `ResolveLeadWorkflow` with precedence: manual override > auto assignment rules > organization default.
- Added resolver unit tests in `internal/identity/service/workflow_resolver_test.go` covering:
  - optional-field matching behavior,
  - assignment-rule matching,
  - manual override resolution paths (including clear mode and missing workflow edge-case).
- Added resolver behavior spec in `docs/workflow-resolver-behavior.md` (precedence, matching semantics, fallback behavior, determinism rules).
- Sonar-first check executed on changed files before implementation; no reported issues after changes.
- Started M3 with initial multi-channel runtime slice:
  - extended `email.Sender` contract with `SendCustomEmail(...)` in `internal/email/brevo.go` and `internal/email/smtp.go`,
  - added generic `email_send` outbox payload handling in `internal/notification/module.go`,
  - extended outbox due processor routing to support `kind=email` with template `email_send` (in addition to existing whatsapp flow).
- Sonar-first check executed for M3 target files before and after implementation; one duplicated-string warning was fixed via shared constant.
- Re-ran quality gates after M3 startup slice:
  - `go build ./...` ✅
  - `go test ./...` ✅
  - `golangci-lint run` ✅ (0 issues)
- Re-ran quality gates after M2 start:
  - `go build ./...` ✅
  - `go test ./...` ✅
  - `golangci-lint run` ✅ (0 issues)
- Implemented M3 step-executor base in `internal/notification/module.go`:
  - added generic workflow-step scheduling (`enqueueWorkflowSteps`) with deterministic step-order execution,
  - added per-channel outbox dispatch for `whatsapp_send` and `email_send`,
  - added recipient-matrix resolution (`includeLeadContact`, `includePartner`, `customPhones`, `customEmails`) with de-duplication,
  - added step-template rendering helpers for subject/body payload materialization.
- Wired existing runtime paths to the new executor base (while preserving behavior):
  - `enqueueLeadWelcomeOutbox`,
  - `enqueueQuoteSentOutbox`,
  - `enqueueAppointmentOutbox`.
- Sonar/Problems first check surfaced maintainability warnings during refactor (cognitive complexity, parameter-count, and unused-symbols); all were resolved in the same change set.
- Re-ran quality gates after M3 step-executor wiring:
  - `go build ./...` ✅
  - `go test ./...` ✅
  - `golangci-lint run ./...` ✅ (0 issues)
- Started M4 quote-override integration:
  - added workflow-aware quote terms resolver adapter `internal/adapters/quote_terms_resolver.go`,
  - wired resolver into quote creation paths in `internal/quotes/service/service.go` so default validity uses effective workflow terms,
  - wired resolver into acceptance PDF data in `internal/adapters/quote_acceptance_processor.go` so payment/valid days in generated PDFs follow effective workflow terms,
  - updated composition root wiring in `cmd/api/main.go`.
- Re-ran quality gates after M4 quote-override slice:
  - `go build ./...` ✅
  - `go test ./...` ✅
  - `golangci-lint run ./...` ✅ (0 issues)
- Implemented M3 retry/failure handling policy for outbox delivery:
  - added delayed retry scheduling in `internal/notification/outbox/repository.go` (`ScheduleRetry`),
  - added bounded retry orchestration in `internal/notification/module.go` with exponential backoff and max-attempt cap,
  - retained immediate-fail behavior for invalid payload/non-retryable parse failures.
- Added runtime behavior documentation in `docs/workflow-runtime-sequence-behavior.md` including:
  - step ordering,
  - recipient matrix semantics,
  - outbox lifecycle,
  - retry policy (backoff + exhaustion behavior).
- Completed M4 communication-path integration for quote events (`quote_sent`, `quote_accepted`, `quote_rejected`):
  - notification routing now resolves workflow rules per channel/audience and enqueues through step executor where configured,
  - explicit non-workflow fallback retained for legacy channels (quote sent email, quote accepted customer/agent emails),
  - disabled workflow rules now suppress their channel path deterministically.
- Enriched quote domain events with lead contact payload for communication routing:
  - `events.QuoteAccepted` now includes `consumerPhone`,
  - `events.QuoteRejected` now includes `consumerEmail`, `consumerName`, `consumerPhone`, `organizationName`.
- Updated quote service event publishing to include contact enrichment for rejected/accepted flows.
- Added focused notification tests in `internal/notification/module_test.go` covering fallback/suppression behavior for quote communication routing.
- Re-ran M4 quality gates after communication integration:
  - `go build ./...` ✅
  - `go test ./...` ✅
  - `golangci-lint run ./...` ✅ (0 issues)
- Completed M5 backend handover pack documentation:
  - `docs/workflow-engine-backend-api-contract.md`
  - `docs/workflow-engine-frontend-integration-guide.md`
  - `docs/workflow-engine-rollout-migration-guide.md`
  - `docs/workflow-engine-known-limitations-followups.md`
- Re-ran quality gates after M5 docs update:
  - `go build ./...` ✅
  - `go test ./...` ✅
  - `golangci-lint run ./...` ✅ (0 issues)

### Open M1 items
- None.
