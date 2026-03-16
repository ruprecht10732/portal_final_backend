# Navigation Link

## Purpose

Provide a navigation link for the resolved accepted job or appointment.

## Use When

- The partner asks for the address, route, navigation, or where they need to go.

## Workflow

1. Resolve the correct accepted job or appointment first.
2. Use `GetNavigationLink` once the lead or appointment target is clear.
3. Return the link in a short Dutch WhatsApp reply without extra system detail.

## Failure Policy

- If the target job is not yet resolved, ask one short question to identify it.
- If no navigation link is available, say so plainly and fall back to the known destination details only if a tool already returned them.

## Safety Boundary

- Never fabricate addresses or links.
- Never provide navigation for jobs outside the current partner scope.