# Skill: JobResolution

## Purpose

Resolve which accepted partner job or appointment the incoming WhatsApp message refers to.

## Workflow

1. Use `GetMyJobs` when the partner asks about their work in general or has not yet identified a specific job.
2. Use `GetPartnerJobDetails` once the partner refers to one job, one appointment, one lead, or one service.
3. Only proceed to write actions after the target appointment or job is unambiguous.

## Resolution Rules

- If one job is clearly implied by the latest exchange, continue with that job.
- If multiple jobs could match, ask one short disambiguating question.
- If no accepted job matches, say so plainly and do not fabricate alternatives.

## Safety Boundary

- Never access or discuss jobs outside the current partner scope.
- Never fall back to organization-wide customer data.