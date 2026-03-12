# Calculator Integration

## Trigger

- Pipeline stage changed to `Estimation`
- Explicit quote generation or repair flow requests

## Preconditions

- Intake is complete enough for bounded pricing.
- Scope can be committed or recovered from trusted context.

## Outputs

- Scope artifacts
- Estimates
- Draft quotes
- Critiques and quote repairs

## Downstream Effects

- Quote sent can move the service to `Proposal`.
- Quote acceptance can move the service to `Fulfillment`, which can wake Matchmaker.