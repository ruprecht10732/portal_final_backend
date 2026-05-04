# Agent Runtime Flow

This page describes the actual backend agent chain as implemented today.

## Entry Events

### Lead Creation And Service Creation

- `LeadCreated`
- `LeadServiceAdded`

These events try to start an initial Gatekeeper run via the unified `AgentTaskScheduler`.

### Attachment Uploads

- `AttachmentUploaded`

Image attachments trigger photo-analysis scheduling. During initial intake, image presence can defer Gatekeeper until photo analysis concludes.

### Human Data Changes

- `LeadDataChanged`

This can re-trigger Gatekeeper and, for call-log sourced updates, can also enqueue Auditor call-log review.

### Visit Reports

- `VisitReportSubmitted`

This can enqueue Auditor review.

## Unified Agent Runtime (v2.0)

All primary agent workspaces are executed through a single `agent.Runtime` instance:

- `Runtime` holds shared dependencies (repository, event bus, model configs, session service) and constructs the appropriate workspace agent on demand.
- A unified `AgentTaskPayload` routes tasks to the correct workspace:
  - `gatekeeper` — intake validation
  - `calculator` — estimation (`mode=estimator`) or quote generation (`mode=quote-generator`)
  - `matchmaker` — partner matching and offer creation
  - `auditor` — call-log or visit-report auditing (uses `AppointmentID` when set)
- The scheduler exposes one `AgentTaskScheduler` interface (`EnqueueAgentTask`). Legacy scheduler interfaces are deprecated and mapped to the unified queue.
- `leads.Module` implements `LeadAutomationProcessor.ProcessAgentTask`, parsing the unified payload and delegating to the correct synchronous runtime method.

## Primary Runtime Chain

1. Gatekeeper validates intake completeness.
2. If intake is ready, the service can move to `Estimation`.
3. `Estimation` can trigger Calculator-runtime flows.
4. Quote acceptance can move the service to `Fulfillment`.
5. `Fulfillment` can trigger Matchmaker/Dispatcher.

## Photo Analysis Deferral

If a new service already has image attachments, initial Gatekeeper evaluation can be deferred until PhotoAnalyzer completes or fails. This avoids making intake-readiness decisions before visual evidence is available.

## Dedupe And Safety

- Gatekeeper, Estimator, and Dispatcher use short-window dedupe guards.
- Terminal services do not trigger agents.
- Manual intervention suppresses further autonomous Gatekeeper progression.
- The scheduler applies workspace-specific uniqueness TTLs (e.g., 45s for gatekeeper, estimator timeout for calculator) while sharing the single `agent:run` task type.

## Related References

- `agent-workspace-architecture.md`
- `workflow-runtime-sequence-behavior.md`
- `workflow-resolver-behavior.md`
- `../agents/README.md`