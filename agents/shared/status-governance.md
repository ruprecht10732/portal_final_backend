# Status Governance

- Lead status and pipeline stage changes are governed by backend invariants and reconciler logic.
- Agents may request a state change only through the approved tools.
- A stage request that violates backend rules must be treated as blocked, not retried with invented facts.
- Event-driven orchestration remains the authority for downstream agent chaining.