# ADR 0001: Notification workflows via DB outbox + Asynq

Date: 2026-02-12

## Status
Proposed

## Context
We send certain notifications (notably WhatsApp messages) directly from request handlers by spawning goroutines and using `time.Sleep` for delays. This has several issues:

- Not durable: process restarts lose pending sends.
- Poor observability: hard to see what is scheduled / retrying / failed.
- Hard to evolve: adding rules (per-tenant, per-source, per-trigger) becomes scattered logic.
- No consistent retry/backoff policy.

We already use Asynq + Redis for delayed work (e.g., appointment reminders). We also have a scheduler process (`cmd/scheduler`) that is the natural home for background workflows.

## Decision
Adopt a **transactional DB outbox** for notification intents, with dispatch and execution handled by the **scheduler** using **Asynq**.

In short:

1. API writes domain data (e.g., Lead, Appointment) and enqueues an outbox row in the **same DB transaction**.
2. Scheduler periodically polls (or streams) the outbox and enqueues an Asynq task per outbox row.
3. Asynq worker executes the task (send WhatsApp/email), records the result, and updates outbox status.

This pattern ensures atomicity between state changes and side effects.

## Scope (initial)
- WhatsApp notifications triggered by existing domain events (e.g., lead welcome, appointment reminders).
- Delays (e.g., “welcome delay”) are applied by scheduling the Asynq task for a future time.
- Per-tenant configuration is read at execution time (or captured at enqueue time if required).

Out of scope for this ADR:
- Full admin UI for arbitrary workflow rules.
- Cross-channel orchestration beyond WhatsApp/email.

## Outbox model (proposed)
A new table, conceptually:

- `id` UUID PK
- `tenant_id` UUID
- `event_name` TEXT (e.g., `RAC_leads.lead.created`)
- `payload` JSONB (minimal info to perform the notification)
- `kind` TEXT (e.g., `whatsapp`, `email`)
- `template` TEXT (e.g., `lead_welcome`, `appointment_reminder`)
- `run_at` TIMESTAMPTZ (when the task should execute)
- `status` TEXT (`pending`, `enqueued`, `processing`, `succeeded`, `failed`, `dead`)
- `attempts` INT
- `last_error` TEXT NULL
- `created_at`, `updated_at`

## Asynq tasks (proposed)
- `notification:dispatch` (optional: for outbox scanning/enqueueing)
- `notification:send` (the actual send)

Task payload should include the outbox `id` and `tenant_id`. The worker loads the row and executes idempotently.

## Idempotency
Sending must be idempotent to avoid duplicates on retries.

Preferred approach:
- Create a `notification_delivery` (or reuse an existing timeline/notification log) keyed by `(outbox_id)`.
- Worker performs “insert-if-not-exists” before sending.

## Error handling & retries
- Use Asynq retry/backoff for transient failures.
- Mark `failed/dead` in outbox when max retries exceeded.
- Keep a human-readable `last_error`.

## Consequences
### Positive
- Durable scheduling and delayed execution.
- Centralized retries/backoff.
- Clear audit trail of what was supposed to be sent and what happened.
- Cleaner separation: API handles domain state; scheduler handles side effects.

### Negative / trade-offs
- Additional DB table + operational complexity.
- Requires careful idempotency design.
- Some notifications will become eventually consistent (seconds) depending on polling/dispatch.

## Migration strategy
1. Introduce outbox schema + minimal library to write outbox rows.
2. In scheduler, implement outbox dispatcher + `notification:send` worker.
3. Move one notification (lead welcome) from in-process goroutine to outbox+Asynq.
4. Iterate (appointment reminders, quote-related notifications, etc.).

## Notes
The recent fix to suppress the lead welcome for `source=quote_flow` remains valid with this architecture; it should be enforced when generating the outbox row and/or when executing the worker.
