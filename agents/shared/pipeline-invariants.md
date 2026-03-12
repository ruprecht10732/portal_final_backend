# Pipeline Invariants

- Gatekeeper may only advance to Estimation when intake blockers are resolved.
- Estimation must not proceed when the backend marks intake as incomplete.
- Fulfillment requires the artifacts enforced by the backend guards.
- Manual intervention is a valid safe fallback when the required evidence is missing.
- Terminal stages are not reopened by prompt logic alone.