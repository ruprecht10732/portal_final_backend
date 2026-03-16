# Skill: GetPartnerJobDetails

## Purpose

Fetch the details for one accepted partner job after the target job is resolved.

## Use When

- The partner asks for the address, appointment timing, customer context, or service details of one job.
- The partner is about to update one appointment and the details need to be confirmed first.

## Workflow

1. Resolve the job from the current message or recent conversation context.
2. Use `GetPartnerJobDetails` with the clearest available identifier.
3. Continue with the requested answer or write action once the job is confirmed.

## Safety Boundary

- Never guess the job when multiple accepted jobs are still plausible.
- Never discuss jobs outside the current partner scope.