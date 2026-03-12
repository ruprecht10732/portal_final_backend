# Gatekeeper Integration

## Trigger

- Lead created
- Lead service added
- Lead data changed
- Photo analysis completed
- Photo analysis failed

## Preconditions

- Service is not terminal.
- Pipeline stage allows Gatekeeper evaluation.
- If image attachments exist during initial creation, Gatekeeper may defer until photo analysis concludes.

## Outputs

- Analysis record
- Optional factual corrections
- Optional service-type correction
- Pipeline-stage update or safe no-op

## Downstream Effects

- `Estimation` can wake Calculator-runtime flows.
- `Nurturing` keeps the service in clarification.
- `Manual_Intervention` alerts humans and suppresses further autonomous progression.