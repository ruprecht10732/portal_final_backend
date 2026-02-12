# Workflow Engine — Backend API Contract (M5)

## Purpose and scope
This contract documents the backend API surface that frontend clients can rely on after M4 completion.

It covers:
- current HTTP endpoints used for workflow-related notification settings,
- quote/public-quote endpoints impacted by workflow-driven communication,
- request/response guarantees for frontend integration.

It does **not** expose internal-only service contracts (for example, workflow-engine repository/service types) that currently have no HTTP route.

## Base URL and auth
- Base URL: `/api/v1`
- Authenticated user endpoints: under `/api/v1` with bearer JWT
- Admin endpoints: under `/api/v1/admin` with bearer JWT + admin role
- Public quote endpoints: under `/api/v1/public/quotes` (no JWT)

Required auth header (protected/admin):
- `Authorization: Bearer <token>`

## Response conventions
- Success: endpoint-specific JSON body, usually HTTP `200`
- Created: HTTP `201` for create operations
- Validation/semantic errors: HTTP `400`
- Unauthorized: HTTP `401`
- Forbidden: HTTP `403`
- Not found: HTTP `404`

Standard error shape:

```json
{
  "error": "message",
  "details": "optional details"
}
```

## Workflow engine settings API (frontend-active)
These routes are used by frontend workflow settings screens.

### GET `/admin/organizations/me/workflow-engine/workflows`
Returns workflow profiles for the current organization.

Response:

```json
{
  "workflows": [
    {
      "trigger": "quote_sent",
      "channel": "whatsapp",
      "audience": "lead",
      "enabled": true,
      "delayMinutes": 5,
      "leadSource": "google_ads",
      "templateText": "Hi {{consumerName}}, je offerte staat klaar"
    }
  ]
}
```

### PUT `/admin/organizations/me/workflow-engine/workflows`
Replaces the organization workflow profiles atomically.

Request:

```json
{
  "workflows": [
    {
      "trigger": "quote_sent",
      "channel": "whatsapp",
      "audience": "lead",
      "enabled": true,
      "delayMinutes": 5,
      "leadSource": "google_ads",
      "templateText": "Hi {{consumerName}}, je offerte staat klaar"
    }
  ]
}
```

Response shape is the same as `GET`.

### GET `/admin/organizations/me/workflow-engine/assignment-rules`
Returns assignment rules for the current organization.

### PUT `/admin/organizations/me/workflow-engine/assignment-rules`
Replaces assignment rules atomically.

## Quote endpoints relevant to workflow communication behavior

### POST `/quotes/:id/send` (authenticated)
- Sends quote proposal flow.
- Triggers quote-sent communication handling (`quote_sent`) in notification runtime.

### POST `/public/quotes/:token/accept` (public)
- Accepts quote and signs.
- Triggers quote-accepted communication handling (`quote_accepted`) in notification runtime.

### POST `/public/quotes/:token/reject` (public)
- Rejects quote.
- Triggers quote-rejected communication handling (`quote_rejected`) in notification runtime.

### GET `/public/quotes/:token/events` (public SSE)
- Server-Sent Events stream for real-time quote updates on public quote pages.

## Runtime behavior contract for quote communication triggers
This is a behavioral contract consumed indirectly by frontend user flows.

For each trigger/channel/audience:
1. Backend resolves workflow rule by `(trigger, channel, audience)`.
2. If no rule exists, backend applies legacy behavior for that path (if a legacy path exists).
3. If rule exists and `enabled=false`, backend suppresses that channel path.
4. If rule exists and `enabled=true`, backend enqueues via outbox workflow runtime.

M4 guarantees for quote flows:
- `quote_sent`:
  - workflow-aware for lead email/whatsapp where configured,
  - fallback to legacy proposal-email sender when enabled-rule enqueue fails or no rule exists.
- `quote_accepted`:
  - workflow-aware for lead email, lead whatsapp, and agent/partner email paths,
  - fallback to legacy thank-you/agent emails where legacy path exists.
- `quote_rejected`:
  - workflow-driven communication path,
  - no-rule behavior remains the previous default (no direct outbound message).

## Additional workflow-engine endpoints
- `GET /admin/organizations/me/workflow-engine/leads/:leadID/override`
- `PUT /admin/organizations/me/workflow-engine/leads/:leadID/override`
- `DELETE /admin/organizations/me/workflow-engine/leads/:leadID/override`
- `GET /admin/organizations/me/workflow-engine/leads/:leadID/resolve`

## Compatibility statement
- Legacy `/admin/organizations/me/workflows` endpoints are removed in strict cutover.
- Runtime configuration is workflow-engine only.
