# Pipeline Invariants

- Gatekeeper may only advance to Estimation when intake blockers are resolved.
- Estimation must not proceed when the backend marks intake as incomplete.
- Fulfillment requires the artifacts enforced by the backend guards.
- Manual intervention is a valid safe fallback when the required evidence is missing.
- Terminal stages are not reopened by prompt logic alone.

[ALLOWED TRANSITIONS]
- Triage -> Nurturing OR Estimation OR Manual_Intervention OR Disqualified
- Nurturing -> Estimation OR Manual_Intervention OR Disqualified
- Estimation -> Proposal OR Nurturing OR Manual_Intervention
- Proposal -> Fulfillment OR Nurturing OR Manual_Intervention
- Fulfillment -> Completed OR Manual_Intervention
- Manual_Intervention -> ANY (human override)
- Disqualified -> Triage (rare reactivation)

[ENFORCEMENT RULES]
NEVER skip stages. NEVER move from Triage directly to Proposal or Fulfillment.
When in doubt, prefer Nurturing or Manual_Intervention as safe fallbacks.
When markdown guidance and Go backend invariants conflict, Go is always authoritative.