# Agent Runtime Flow

This page describes the actual backend agent chain as implemented today.

## Entry Events

### Lead Creation And Service Creation

- `LeadCreated`
- `LeadServiceAdded`

These events try to start an initial Gatekeeper run.

### Attachment Uploads

- `AttachmentUploaded`

Image attachments trigger photo-analysis scheduling. During initial intake, image presence can defer Gatekeeper until photo analysis concludes.

### Human Data Changes

- `LeadDataChanged`

This can re-trigger Gatekeeper and, for call-log sourced updates, can also enqueue Auditor call-log review.

### Visit Reports

- `VisitReportSubmitted`

This can enqueue Auditor review.

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

## Related References

- `agent-workspace-architecture.md`
- `workflow-runtime-sequence-behavior.md`
- `workflow-resolver-behavior.md`
- `../agents/README.md`