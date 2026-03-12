# Matchmaker Integration

## Trigger

- Pipeline stage changed to `Fulfillment`
- Partner offer rejected and a replacement partner is needed

## Preconditions

- Auto-dispatch is enabled for the tenant.
- No active or accepted partner flow already blocks redispatch.

## Outputs

- Matching partner search
- Partner offer creation
- Fulfillment routing stage updates

## Downstream Effects

- Partner offer workflows and offer summaries
- Manual intervention when no eligible partner is found