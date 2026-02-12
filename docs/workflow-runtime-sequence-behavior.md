# Workflow Runtime Sequence Behavior (M3)

## Purpose
This document describes how workflow steps are converted into outbox jobs, executed, and retried.

## Key Terms (concise)
- Step executor: component that turns workflow steps into scheduled outbox records.
- Outbox: database queue table (`RAC_notification_outbox`) used for reliable async delivery.
- Backoff: increasing wait time between retries after failures.
- Retry exhaustion: state where max retry attempts are reached and a job is marked failed.

## Step scheduling flow
1. Runtime receives a domain event (for example lead welcome, quote sent, appointment reminders).
2. Runtime builds one or more workflow steps.
3. Step executor orders steps deterministically:
   - primary key: `step_order` ascending,
   - tie-break: `created_at` ascending.
4. For each enabled step:
   - render template body/subject with variables,
   - resolve recipients from recipient config,
   - create outbox records by channel:
     - WhatsApp => `kind=whatsapp`, `template=whatsapp_send`
     - Email => `kind=email`, `template=email_send`

## Recipient matrix behavior
Resolved from `recipient_config`:
- `includeLeadContact`: include lead phone/email when available.
- `includePartner`: include partner phone/email when available.
- `customPhones` / `customEmails`: append custom recipients.
- De-duplication is applied case-insensitively.

## Outbox delivery flow
1. Dispatcher claims pending records and enqueues scheduler tasks.
2. Notification handler marks a record `processing` (attempt count increments).
3. Channel handler executes send logic.
4. On success: record is marked `succeeded`.

## Retry/failure policy
- Max attempts: `5`.
- Backoff formula: `1m, 2m, 4m, 8m, 16m` (capped at 60m).
- On transient delivery error:
  - if attempts remaining: status set back to `pending`, `run_at` moved forward by backoff delay,
  - if max attempts reached: status set to `failed`.
- On non-retryable payload/config errors (invalid payload): record is marked `failed` immediately.

## Safety and observability
- Logs are written for:
  - retry scheduling (with attempt and next retry time),
  - retry exhaustion,
  - retry scheduling failure fallback.
- Existing behavior for unsupported kind/template remains fail-fast with explicit reason.

## Current runtime integration scope
Current M3 wiring uses the step executor base for:
- lead welcome enqueue path,
- quote sent enqueue path,
- appointment enqueue path.

Further trigger families can be migrated onto the same executor pattern without changing outbox delivery semantics.

## Quote communication integration (M4)
Quote communication paths now support workflow-aware routing per trigger/channel with explicit fallback semantics:
- Triggers: `quote_sent`, `quote_accepted`, `quote_rejected`.
- Channels: `email` and `whatsapp` (lead-facing), and `email` for partner/agent notifications on `quote_accepted`.

Routing behavior:
1. Resolve workflow rule by `(trigger, channel, audience)`.
2. If no rule exists, execute legacy behavior (non-workflow fallback).
3. If a rule exists and is disabled, suppress that channel.
4. If a rule exists and is enabled:
  - enqueue via step executor/outbox,
  - if enqueue fails, fall back to legacy behavior for channels that had a legacy path.

Legacy fallback paths retained:
- `quote_sent` lead email fallback (proposal email sender).
- `quote_accepted` lead email fallback (thank-you with PDF attachment).
- `quote_accepted` agent email fallback.

`quote_rejected` communication is workflow-driven; when no rule exists, prior behavior (no direct outbound communication) remains unchanged.
