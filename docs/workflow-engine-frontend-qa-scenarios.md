# Workflow Engine — Frontend QA Scenarios (M5)

## Purpose
Compact manual QA script for frontend validation after backend workflow-engine rollout.

## Preconditions
- Backend API + scheduler are running.
- Test tenant exists with valid admin credentials.
- At least one quote exists with valid public token.
- Frontend points to the target backend environment.

## Scenario 1 — Load workflow settings
**Goal:** confirm current settings API contract remains compatible.

- Open workflow settings page in frontend.
- Expect settings to load successfully from `GET /api/v1/admin/organizations/me/workflow-engine/workflows`.
- Verify no schema/runtime errors in browser console.

**Pass criteria**
- UI renders current trigger cards.
- Existing values (`enabled`, `delayMinutes`, optional template/source) are shown.

## Scenario 2 — Save workflow settings
**Goal:** confirm replace semantics and validation boundaries.

- Update one trigger card (for example `quote_sent`):
  - toggle `enabled`
  - set `delayMinutes` within `0..1440`
  - optionally set `templateText`
- Save settings.
- Refresh page.

**Pass criteria**
- Save succeeds via `PUT /api/v1/admin/organizations/me/workflow-engine/workflows`.
- Values persist after refresh.
- Invalid values (for example delay outside range) return a user-visible error.

## Scenario 3 — Quote send UX is action-based, not delivery-based
**Goal:** ensure FE treats quote action success independently from async delivery.

- Trigger send quote action in frontend (`POST /api/v1/quotes/:id/send`).
- Observe immediate UX response.

**Pass criteria**
- UI shows quote send success when API call succeeds.
- UI does **not** wait for email/whatsapp completion confirmation.

## Scenario 4 — Public quote accept flow
**Goal:** verify public acceptance path remains stable.

- Open public quote page with valid token.
- Accept quote (`POST /api/v1/public/quotes/:token/accept`).

**Pass criteria**
- Accept action succeeds and user-facing success state is shown.
- Page state updates correctly after acceptance.

## Scenario 5 — Public quote reject flow
**Goal:** verify public rejection path remains stable.

- Open public quote page with valid token.
- Reject quote (`POST /api/v1/public/quotes/:token/reject`).

**Pass criteria**
- Reject action succeeds and user-facing rejection state is shown.
- No frontend regression in quote page state handling.

## Scenario 6 — Public quote SSE stream
**Goal:** verify SSE contract still supports real-time updates.

- Open a public quote page that subscribes to `GET /api/v1/public/quotes/:token/events`.
- In parallel, perform a quote action that emits updates.

**Pass criteria**
- SSE connection establishes successfully.
- Client receives and applies relevant updates without page reload.

## Scenario 7 — Rule-disabled suppression behavior (frontend expectation)
**Goal:** validate expected UX when a workflow rule is disabled.

- Disable a relevant workflow rule in settings (for example `quote_sent` + whatsapp).
- Execute matching quote action.

**Pass criteria**
- Quote action still succeeds in UI.
- No FE assumption that channel message must be delivered.

## Scenario 8 — Error payload handling
**Goal:** ensure frontend error display matches backend shape.

- Trigger a controlled `400` (invalid payload) and a `403` (insufficient permission).

**Pass criteria**
- Frontend displays backend `error` field as primary message.
- Optional `details` is handled gracefully (debug/secondary display only).

## Quick sign-off checklist
- [ ] Workflow settings load/save is stable
- [ ] Quote send/accept/reject flows show correct success/error states
- [ ] Public quote SSE updates are received and rendered
- [ ] No FE dependency on non-exposed workflow-engine foundation APIs
