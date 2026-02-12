# Workflow Engine — Frontend Integration Guide (M5)

## Goal
Provide frontend teams with a safe integration path for workflow-related behavior after backend M4.

## Current integration boundary
Use these backend APIs for workflow configuration:
- `GET /api/v1/admin/organizations/me/workflow-engine/workflows`
- `PUT /api/v1/admin/organizations/me/workflow-engine/workflows`
- `GET /api/v1/admin/organizations/me/workflow-engine/assignment-rules`
- `PUT /api/v1/admin/organizations/me/workflow-engine/assignment-rules`

## Existing frontend service mapping
Current web app maps to workflow-engine contracts:
- `OrganizationService.getWorkflowEngineWorkflows()`
- `OrganizationService.replaceWorkflowEngineWorkflows(...)`
- `OrganizationService.getWorkflowAssignmentRules()`
- `OrganizationService.replaceWorkflowAssignmentRules(...)`

## Supported trigger set for current UI
The existing organization settings UI uses these workflow triggers:
- `lead_welcome`
- `quote_sent`
- `appointment_created`
- `appointment_reminder`
- `partner_offer_created`

Current card implementation stores WhatsApp-oriented rules (`channel = whatsapp`) with per-trigger:
- `enabled`
- `delayMinutes`
- optional `leadSource`
- optional `templateText`

## Quote-flow behavior expectations in UX
Frontend quote UX should assume communication side effects happen asynchronously in backend after these calls:
- `POST /api/v1/quotes/:id/send`
- `POST /api/v1/public/quotes/:token/accept`
- `POST /api/v1/public/quotes/:token/reject`

Implications for UI:
- treat response success as business action success (send/accept/reject), not guaranteed immediate message delivery,
- do not block UI waiting for email/whatsapp delivery completion,
- show user-facing status around quote action completion, not channel delivery confirmation.

## SSE usage
For public quote pages, use:
- `GET /api/v1/public/quotes/:token/events`

Purpose:
- real-time quote state changes on proposal page while backend processes updates.

## Error handling guidance
Backend error payload shape:

```json
{
  "error": "message",
  "details": "optional"
}
```

Recommended frontend behavior:
- show `error` as primary fallback message,
- inspect `details` only for advanced/debug UI,
- treat `400/403` as user-correctable/permission issues, `500` as retryable system issues.

## Payload guidance for workflow settings screen
When sending `PUT /admin/organizations/me/workflow-engine/workflows`:
- always send full replacement array (`workflows`),
- keep `delayMinutes` within `0..525600`,
- trim optional template fields before sending,
- keep recipient config aligned to audience.

## Non-goals for this handover iteration
Per-lead override and resolver preview are available under `/workflow-engine/leads/:leadId/*`.

## Frontend validation checklist
- Workflow settings page can load and save rules with no schema mismatch.
- Quote send/accept/reject flows remain stable and do not regress.
- Public quote SSE still connects and updates state.
- No frontend dependency is added on non-exposed workflow-engine endpoints.
