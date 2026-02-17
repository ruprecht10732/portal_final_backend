# ADR 0003: Context-Aware LeadService State Reconciliation Engine

Date: 2026-02-17
Status: Proposed

## Context

`RAC_lead_services` stores a mutable, user-facing state pair:

- `pipeline_stage` (lifecycle stage; enum includes values like `Triage`, `Quote_Draft`, `Quote_Sent`, `Completed`, `Lost`)
- `status` (service status; values include `New`, `Appointment_Scheduled`, `Quote_Sent`, `Completed`, `Lost`, `Disqualified`, …)

Today, these can become inconsistent with the *actual* underlying child entities that represent work performed on a service. This primarily happens when a child entity is deleted or materially changed after the service was moved forward.

Examples:

- A quote is deleted, but the lead service stays in `Quote_Draft` or `Quote_Sent`.
- An appointment is cancelled / rescheduled, but `status` remains `Appointment_Scheduled`.
- A service is marked terminal (`Completed`, `Lost`, `Disqualified`) but later receives new “active” activity (e.g., a new quote or a new appointment), and the system cannot “resurrect” the service safely.

This causes:

- Incorrect UI / exports / reporting (pipeline and status are treated as authoritative).
- Orchestrator behavior mismatches (AI agents skip terminal services via `domain.IsTerminal`).
- Operational confusion: when the system auto-corrects state today (if it does at all), there is no transparent timeline entry explaining why.

### Requirements

- **Single source of truth** for `pipeline_stage` and `status` must be derivable from child entities.
- Must handle:
  - stale drafts via **time-based decay** (default staleDraftDuration: **30 days**)
  - **resurrection** of terminal services when strong active signals appear
  - **transparency**: when the system auto-corrects service state (especially regressions and resurrection), record a lead timeline event with the reason

### Constraints / Inputs

- Reconciliation is triggered **event-driven**, not via periodic polling:
  - QuoteCreated / QuoteDeleted
  - AppointmentStatusChanged
  - LeadDataChanged
- Reconciliation is computed from a single **aggregates query** per service:
  - counts (offers, quotes, etc.)
  - latest timestamps (e.g., last quote updated, last appointment updated)
  - visit report existence
  - AI action existence / latest AI run
- Derivation hierarchy (strongest signal first):
  1. Offers
  2. Quotes
  3. Visit Reports
  4. Appointments
  5. AI
- Terminal states should not be reverted unless there are **strong active signals** (resurrection rule).

## Decision

We will implement a **context-aware LeadService State Reconciliation Engine** inside the leads orchestrator layer.

### DEC-001: Event-driven reconciliation entry points

- **DEC-001**: Reconciliation runs in response to domain events that indicate the service’s child-entity graph may have changed:
  - QuoteCreated
  - QuoteDeleted
  - AppointmentStatusChanged
  - LeadDataChanged
- **DEC-002**: Each handler maps the event to a `lead_service_id` and calls `reconcileServiceState(ctx, leadServiceID, tenantID)`.
- **DEC-003**: Reconciliation is safe to call repeatedly; it must be **idempotent** and **side-effect minimal**.

### DEC-002: Aggregate snapshot query as source of truth

- **DEC-004**: Add repository method `GetServiceStateAggregates(ctx, serviceID)` (scoped by organization/tenant in implementation) that returns a compact snapshot used for derivation.
- **DEC-005**: The query uses the canonical schema tables (`rac_*`) and returns only what the derivation needs:
  - offer counts + latest offer timestamp
  - quote counts by status (Draft/Sent/Accepted/Rejected/Expired) + latest quote timestamp
  - visit report existence + latest visit report timestamp
  - appointment counts by status + latest appointment timestamp
  - latest AI run timestamp (and/or latest AI “action” type)

**Data model / query note (table casing)**

- **DEC-006**: SQL in this repository frequently references `RAC_*` identifiers without quotes; Postgres folds unquoted identifiers to lowercase.
- **DEC-007**: New reconciliation SQL will use **unquoted lowercase** `rac_*` table names consistently (e.g., `rac_quotes`, `rac_appointments`, `rac_partner_offers`) to avoid accidental quoted identifiers and to match the physical schema (`public.rac_*`).

### DEC-003: Deterministic derivation rules and hierarchy

- **DEC-008**: `reconcileServiceState` computes a derived `(pipeline_stage, status)` using a deterministic ruleset with the fixed hierarchy:
  - offers override quotes
  - quotes override visit reports
  - visit reports override appointments
  - appointments override AI

- **DEC-009**: The derivation must be explainable. The reconciler returns a `reason_code` plus key evidence (counts/timestamps) used to make the decision.

Example (illustrative; not exhaustive):

- **Offers present** → ensure `pipeline_stage >= Partner_Matching` (or higher based on accepted/assigned) and set `status` accordingly.
- **Quote accepted** → `status = Quote_Accepted` and `pipeline_stage = Quote_Sent` (or the next stage if partner flow is applicable).
- **Draft quote(s) exist but none sent** → `pipeline_stage = Quote_Draft`, `status = Quote_Draft`.
- **Visit report exists** (and no stronger signals) → `status = Survey_Completed`, `pipeline_stage = Ready_For_Estimator`.
- **Appointment scheduled** (and no stronger signals) → `status = Appointment_Scheduled`, `pipeline_stage = Nurturing` (or current policy).
- **Only AI activity exists** (and no stronger signals) → keep or move to `Triage` / `Nurturing` depending on recency.

### DEC-004: Stale draft decay (time-based)

- **DEC-010**: If the strongest signal is a draft quote (or otherwise “draft-only” service) and the last meaningful activity timestamp is older than `staleDraftDuration` (default **30 days**), the reconciler may decay the state.
- **DEC-011**: Decay must not silently delete data; it only changes `pipeline_stage` / `status` to a less-advanced non-terminal state (e.g., `Nurturing` or `Triage`) depending on existing policy.

### DEC-005: Terminal protection + resurrection rule

- **DEC-012**: If `domain.IsTerminal(current.Status, current.PipelineStage)` is true, reconciliation must not revert to non-terminal **unless** the aggregates contain a strong active signal.
- **DEC-013**: Strong active signals (initial set):
  - a new quote created/updated after the terminal timestamp
  - a new appointment created or moved into an “active” status after the terminal timestamp
  - a new offer created after the terminal timestamp
- **DEC-014**: Resurrection must be explicit in the audit trail (timeline event) and must include the triggering evidence.

### DEC-006: Timeline transparency for auto-corrections

- **DEC-015**: When reconciliation results in a state change, it updates the lead service via existing repository update methods (which already insert into `RAC_lead_service_events`).
- **DEC-016**: Additionally, when the change is:
  - a **regression** (derived stage/status is “earlier” than current), or
  - a **resurrection** (terminal → non-terminal)

  the system writes a lead timeline event into `lead_timeline_events`.

- **DEC-017**: Timeline event conventions for reconciliation transparency:
  - `actor_type`: `System`
  - `actor_name`: `StateReconciler`
  - `event_type`: `service_state_reconciled`
  - `title`: a short human-readable statement (e.g., “Service state auto-corrected”)
  - `summary`: optional; include the reason and key evidence
  - `metadata`: structured evidence

Example metadata keys:

- `reasonCode`: `quote_deleted_regression` | `stale_draft_decay` | `terminal_resurrection_quote` | …
- `before`: `{ "status": "Quote_Sent", "pipelineStage": "Quote_Sent" }`
- `after`: `{ "status": "New", "pipelineStage": "Ready_For_Estimator" }`
- `evidence`: `{ "quoteDraftCount": 0, "quoteSentCount": 0, "offerCount": 0, "lastQuoteAt": null, ... }`
- `trigger`: `{ "event": "quotes.quote.deleted", "eventId": "…" }`

## Consequences

### Positive

- **POS-001**: Restores data integrity: `pipeline_stage`/`status` remain consistent with the real child-entity graph.
- **POS-002**: Makes the workflow self-healing after deletes/updates without requiring manual DB intervention.
- **POS-003**: Enables safe resurrection when a “closed” lead becomes active again, improving operational throughput.
- **POS-004**: Improves auditability: regressions/resurrections produce explicit timeline events with evidence.

### Negative

- **NEG-001**: Adds complexity to the leads orchestrator; needs careful rule maintenance as new child entities are introduced.
- **NEG-002**: Requires consistent event publication across modules; missed events can delay reconciliation.
- **NEG-003**: Derivation rules can be contentious; incorrect hierarchy/mapping can cause surprising state changes.
- **NEG-004**: Additional DB load from aggregate queries on high-frequency event streams (mitigated by idempotency and compact queries).

## Alternatives Considered

### A1: Do nothing (manual repair)

- **ALT-001**: **Description**: Continue with current behavior; rely on humans to correct inconsistent services.
- **ALT-002**: **Rejection Reason**: Unscalable and opaque; state inconsistencies are user-visible and affect orchestration.

### A2: Compute service state on-read only (no stored `pipeline_stage` / `status`)

- **ALT-003**: **Description**: Remove persistence of `pipeline_stage`/`status` and compute on every API read.
- **ALT-004**: **Rejection Reason**: Requires broad API/UI refactors; makes exports and event-ledgers harder; risks performance regressions.

### A3: Nightly batch reconciliation

- **ALT-005**: **Description**: Run a scheduled job that reconciles all services once per day.
- **ALT-006**: **Rejection Reason**: Slow feedback loop; inconsistent state remains visible for hours; poor fit for “resurrection” immediacy.

### A4: Hard-delete prevention / strict FK constraints

- **ALT-007**: **Description**: Reduce deletes/changes by tightening constraints and disallowing certain operations.
- **ALT-008**: **Rejection Reason**: Does not address legitimate deletes/edits; still fails on partial updates and external integrations.

## Rollout Plan

- **ROL-001**: Implement `GetServiceStateAggregates` in leads repository (single query + unit tests around derivation).
- **ROL-002**: Add `reconcileServiceState` to leads orchestrator (pure derivation + idempotent update).
- **ROL-003**: Introduce/ensure domain events exist and are published:
  - QuoteCreated / QuoteDeleted from quotes module
  - AppointmentStatusChanged from appointments module
  - LeadDataChanged is already present (`leads.data.changed`)
- **ROL-004**: Subscribe orchestrator (or a dedicated reconciler listener) to these events and invoke reconciliation.
- **ROL-005**: Add a feature flag (env/config) to enable reconciliation per tenant or globally.
- **ROL-006**: Run a one-time backfill job (optional) to reconcile recent services (e.g., last 90 days) before fully enabling.

## Observability / Monitoring

- **OBS-001**: Structured logs for every *applied* reconciliation with fields: `leadId`, `serviceId`, `tenantId`, `before`, `after`, `reasonCode`, `triggerEvent`.
- **OBS-002**: Metrics counters:
  - `leadservice_reconcile_runs_total`
  - `leadservice_reconcile_changes_total{type=progression|regression|resurrection|decay}`
  - `leadservice_reconcile_skipped_total{reason=no_change|terminal_no_signal|missing_service}`
- **OBS-003**: Timeline events (`lead_timeline_events`) serve as a user-facing audit trail for regressions/resurrections.
- **OBS-004**: Add a lightweight dashboard panel (optional) based on the above metrics to detect rule churn or event storms.

## Open Questions

- **OPQ-001**: What is the authoritative mapping from aggregates to `status` values (especially around appointment states and visit report semantics)?
- **OPQ-002**: How do we define and persist the “terminal timestamp” used for resurrection comparisons (derive from timeline / service events / updated_at)?
- **OPQ-003**: Should stale-draft decay affect both `pipeline_stage` and `status`, or only `pipeline_stage`?
- **OPQ-004**: Do we need to debounce reconciliation (e.g., coalesce multiple QuoteUpdated events) to reduce DB load?

## References

- **REF-001**: Existing pipeline/timeline schema: `migrations/046_pipeline_and_timeline.sql`
- **REF-002**: Domain terminal rules: `internal/leads/domain/service_state.go`
- **REF-003**: Existing event bus patterns: `internal/events/event.go`
