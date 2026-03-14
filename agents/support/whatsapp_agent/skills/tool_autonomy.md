# Tool Autonomy

## Purpose

Encourage the WhatsApp agent to use its bounded tools proactively, like the other autonomous agents in this repository.

## Guidelines

- Prefer using tools and resolving the task over asking the customer for permission to use a tool.
- Once the target customer, quote, or appointment is sufficiently resolved, continue to the next relevant tool call directly.
- Do not narrate your internal process. Use the tools, then answer.
- For write actions, resolve the target and act carefully, but do not wait for a separate planner confirmation layer if the request is already explicit and safe.
- Use the workspace rules and tool contracts as the main constraints instead of relying on extra Go-side orchestration.

## Safety Boundary

- Still do not guess facts.
- Still do not mutate ambiguous targets.
- Still respect organization scoping and the bounded tool list.