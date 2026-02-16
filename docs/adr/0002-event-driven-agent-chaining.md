# ADR 0002: Event-Driven Agent Chaining

Date: 2026-02-16
Status: Accepted

## Context
Our previous architecture coupled AI agents (Gatekeeper, PhotoAnalyzer) loosely via the database but lacked a coherent signaling mechanism.
1. `Gatekeeper` ran on lead creation.
2. `PhotoAnalyzer` ran on file upload.
3. `Gatekeeper` had no access to `PhotoAnalysis` results because it usually ran before the analysis was complete, or simply did not check the database for it.
4. This resulted in the Gatekeeper incorrectly moving leads to `Nurturing` (asking for info) when that info was clearly visible in photos.

## Decision
We are adopting an event-driven agent chaining pattern. Agents will no longer run fire-and-forget based only on HTTP requests. Instead:

1. Domain events for AI completion: when an AI task finishes (for example `PhotoAnalysis`), it publishes a typed domain event (for example `events.PhotoAnalysisCompleted`).
2. Orchestrator as listener: the Orchestrator subscribes to these specific completion events.
3. Contextual re-evaluation: the Orchestrator determines if a downstream or upstream agent should re-run based on new data.
   - Example: when `PhotoAnalysisCompleted` fires, the Orchestrator wakes the Gatekeeper to re-triage the lead with visual context.
4. Data injection: agents explicitly fetch peer-agent outputs from the repository at run start, and prompt builders include these summaries.

## Technical Changes
1. Introduced `events.PhotoAnalysisCompleted`.
2. Updated `PhotoAnalysisHandler` to publish this event upon successful analysis.
3. Updated `Orchestrator` to listen to this event and trigger `Gatekeeper` if the lead is in `Triage` or `Nurturing`.
4. Updated `Gatekeeper` to fetch `repository.PhotoAnalysis` and inject it into the prompt.

## Consequences
### Positive
- Higher accuracy: Gatekeeper now sees what the PhotoAnalyzer sees.
- Self-correcting pipeline: if a lead is initially marked missing information, arrival of photo analysis can automatically move it to `Ready_For_Estimator`.
- Reduced hallucinations: agents rely on structured peer data rather than guessing.

### Negative / Trade-offs
- Latency: there is a delay between photo upload and Gatekeeper re-evaluation (asynchronous process).
- Complexity: debugging requires tracing events through the bus instead of following a purely linear imperative flow.

## Optimization Notes
To reduce API calls:
- Do not trigger Gatekeeper immediately on `AttachmentUploaded`.
- Wait for `PhotoAnalysisCompleted` so Gatekeeper runs once with high-value visual data instead of multiple times during upload bursts.
