# Lead Energy Label Backfill

Run the backfill to fetch EP-Online energy labels for legacy leads that were created before enrichment was introduced.

```
go run ./cmd/lead-energylabel-backfill
```

Environment requirements:

- `DATABASE_URL`, `JWT_ACCESS_SECRET`, and `JWT_REFRESH_SECRET` must be set (config loader validation).
- `EP_ONLINE_API_KEY` must be present; otherwise the command exits immediately.

The command processes batches of 25 leads missing `energy_label_fetched_at`, throttles requests to protect the EP-Online API, and logs progress for each lead.
