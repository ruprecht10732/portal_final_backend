-- Quotes Domain SQL Queries

-- name: NextQuoteNumber :one
INSERT INTO RAC_quote_counters (organization_id, last_number)
VALUES ($1, 1)
ON CONFLICT (organization_id) DO UPDATE SET last_number = RAC_quote_counters.last_number + 1
RETURNING last_number;

-- name: CreateQuote :exec
INSERT INTO RAC_quotes (
  id, organization_id, lead_id, lead_service_id, created_by_id, quote_number, status,
  pricing_mode, discount_type, discount_value,
  subtotal_cents, discount_amount_cents, tax_total_cents, total_cents,
  valid_until, notes, financing_disclaimer, created_at, updated_at,
  public_token, public_token_expires_at, preview_token, preview_token_expires_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23);

-- name: UpdateQuoteWithItems :execrows
UPDATE RAC_quotes SET
  pricing_mode = $2,
  discount_type = $3,
  discount_value = $4,
  subtotal_cents = $5,
  discount_amount_cents = $6,
  tax_total_cents = $7,
  total_cents = $8,
  valid_until = $9,
  notes = $10,
  financing_disclaimer = $11,
  updated_at = $12
WHERE id = $1 AND organization_id = $13;

-- name: DeleteQuoteItemsByQuote :exec
DELETE FROM RAC_quote_items WHERE quote_id = $1 AND organization_id = $2;

-- name: CreateQuoteItem :exec
INSERT INTO RAC_quote_items (
  id, quote_id, organization_id, title, description, quantity, quantity_numeric,
  unit_price_cents, tax_rate, is_optional, is_selected, sort_order, catalog_product_id, created_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14);

-- name: GetQuoteByID :one
SELECT q.id, q.organization_id, q.lead_id, q.lead_service_id, q.created_by_id,
  u.first_name, u.last_name, u.email,
  l.consumer_first_name, l.consumer_last_name, l.consumer_phone, l.consumer_email,
  l.address_street, l.address_house_number, l.address_zip_code, l.address_city,
  q.quote_number, q.status,
  q.pricing_mode, q.discount_type, q.discount_value,
  q.subtotal_cents, q.discount_amount_cents, q.tax_total_cents, q.total_cents,
  q.valid_until, q.notes, q.created_at, q.updated_at,
  q.public_token, q.public_token_expires_at, q.preview_token, q.preview_token_expires_at,
  q.viewed_at, q.accepted_at, q.rejected_at,
  q.rejection_reason, q.signature_name, q.signature_data, q.signature_ip, q.pdf_file_key,
  q.financing_disclaimer
FROM RAC_quotes q
LEFT JOIN RAC_users u ON u.id = q.created_by_id
LEFT JOIN RAC_leads l ON l.id = q.lead_id AND l.organization_id = q.organization_id
WHERE q.id = $1 AND q.organization_id = $2;

-- name: GetLatestNonDraftByLead :one
SELECT id, organization_id, lead_id, lead_service_id, quote_number, status, total_cents, public_token, pdf_file_key
FROM RAC_quotes
WHERE lead_id = $1 AND organization_id = $2 AND status != 'Draft'
ORDER BY created_at DESC
LIMIT 1;

-- name: ListQuoteItemsByQuoteID :many
SELECT id, quote_id, organization_id, title, description, quantity, quantity_numeric,
  unit_price_cents, tax_rate, is_optional, is_selected, sort_order, catalog_product_id, created_at
FROM RAC_quote_items
WHERE quote_id = $1 AND organization_id = $2
ORDER BY sort_order ASC;

-- name: ListQuoteItemsByQuoteIDs :many
SELECT id, quote_id, organization_id, title, description, quantity, quantity_numeric,
  unit_price_cents, tax_rate, is_optional, is_selected, sort_order, catalog_product_id, created_at
FROM RAC_quote_items
WHERE organization_id = $1 AND quote_id = ANY(sqlc.arg(quote_ids)::uuid[])
ORDER BY quote_id, sort_order ASC;

-- name: UpdateQuoteStatus :execrows
UPDATE RAC_quotes SET status = $3, updated_at = $4 WHERE id = $1 AND organization_id = $2;

-- name: SetQuoteLeadServiceID :execrows
UPDATE RAC_quotes q
SET lead_service_id = $3,
  updated_at = $4
FROM RAC_lead_services ls
WHERE q.id = $1
  AND q.organization_id = $2
  AND ls.id = $3
  AND ls.organization_id = $2
  AND ls.lead_id = q.lead_id;

-- name: ValidateQuoteLeadServiceID :one
SELECT EXISTS (
  SELECT 1
  FROM RAC_quotes q
  JOIN RAC_lead_services ls
    ON ls.id = $3
   AND ls.organization_id = q.organization_id
   AND ls.lead_id = q.lead_id
  WHERE q.id = $1 AND q.organization_id = $2
) AS exists;

-- name: DeleteQuote :execrows
DELETE FROM RAC_quotes WHERE id = $1 AND organization_id = $2;

-- name: CountQuotes :one
SELECT COUNT(DISTINCT q.id)
FROM RAC_quotes q
LEFT JOIN RAC_leads l ON l.id = q.lead_id AND l.organization_id = q.organization_id
LEFT JOIN RAC_users u ON u.id = q.created_by_id
WHERE q.organization_id = sqlc.arg('organization_id')::uuid
  AND (sqlc.narg('lead_id')::uuid IS NULL OR q.lead_id = sqlc.narg('lead_id')::uuid)
  AND (sqlc.narg('status')::text IS NULL OR q.status::text = sqlc.narg('status')::text)
  AND (sqlc.narg('search')::text IS NULL OR (
    q.quote_number ILIKE sqlc.narg('search')::text OR q.notes ILIKE sqlc.narg('search')::text
    OR l.consumer_first_name ILIKE sqlc.narg('search')::text OR l.consumer_last_name ILIKE sqlc.narg('search')::text
    OR l.consumer_phone ILIKE sqlc.narg('search')::text OR l.consumer_email ILIKE sqlc.narg('search')::text
    OR l.address_street ILIKE sqlc.narg('search')::text OR l.address_house_number ILIKE sqlc.narg('search')::text
    OR l.address_zip_code ILIKE sqlc.narg('search')::text OR l.address_city ILIKE sqlc.narg('search')::text
    OR u.first_name ILIKE sqlc.narg('search')::text OR u.last_name ILIKE sqlc.narg('search')::text OR u.email ILIKE sqlc.narg('search')::text
    OR EXISTS (
      SELECT 1
      FROM RAC_quote_items qi
      WHERE qi.quote_id = q.id AND qi.organization_id = q.organization_id
        AND qi.description ILIKE sqlc.narg('search')::text
    )
  ))
  AND (sqlc.narg('created_at_from')::timestamptz IS NULL OR q.created_at >= sqlc.narg('created_at_from')::timestamptz)
  AND (sqlc.narg('created_at_to')::timestamptz IS NULL OR q.created_at < sqlc.narg('created_at_to')::timestamptz)
  AND (sqlc.narg('valid_until_from')::timestamptz IS NULL OR q.valid_until >= sqlc.narg('valid_until_from')::timestamptz)
  AND (sqlc.narg('valid_until_to')::timestamptz IS NULL OR q.valid_until < sqlc.narg('valid_until_to')::timestamptz)
  AND (sqlc.narg('total_from')::bigint IS NULL OR q.total_cents >= sqlc.narg('total_from')::bigint)
  AND (sqlc.narg('total_to')::bigint IS NULL OR q.total_cents <= sqlc.narg('total_to')::bigint);

-- name: ListQuotes :many
SELECT q.id, q.organization_id, q.lead_id, q.lead_service_id,
  q.created_by_id, u.first_name, u.last_name, u.email,
  l.consumer_first_name, l.consumer_last_name, l.consumer_phone, l.consumer_email,
  l.address_street, l.address_house_number, l.address_zip_code, l.address_city,
  q.quote_number, q.status, q.pricing_mode, q.discount_type, q.discount_value,
  q.subtotal_cents, q.discount_amount_cents, q.tax_total_cents, q.total_cents,
  q.valid_until, q.notes, q.created_at, q.updated_at,
  q.public_token, q.public_token_expires_at, q.preview_token, q.preview_token_expires_at,
  q.viewed_at, q.accepted_at, q.rejected_at,
  q.rejection_reason, q.signature_name, q.signature_data, q.signature_ip, q.pdf_file_key,
  q.financing_disclaimer
FROM RAC_quotes q
LEFT JOIN RAC_leads l ON l.id = q.lead_id AND l.organization_id = q.organization_id
LEFT JOIN RAC_users u ON u.id = q.created_by_id
WHERE q.organization_id = sqlc.arg('organization_id')::uuid
  AND (sqlc.narg('lead_id')::uuid IS NULL OR q.lead_id = sqlc.narg('lead_id')::uuid)
  AND (sqlc.narg('status')::text IS NULL OR q.status::text = sqlc.narg('status')::text)
  AND (sqlc.narg('search')::text IS NULL OR (
    q.quote_number ILIKE sqlc.narg('search')::text OR q.notes ILIKE sqlc.narg('search')::text
    OR l.consumer_first_name ILIKE sqlc.narg('search')::text OR l.consumer_last_name ILIKE sqlc.narg('search')::text
    OR l.consumer_phone ILIKE sqlc.narg('search')::text OR l.consumer_email ILIKE sqlc.narg('search')::text
    OR l.address_street ILIKE sqlc.narg('search')::text OR l.address_house_number ILIKE sqlc.narg('search')::text
    OR l.address_zip_code ILIKE sqlc.narg('search')::text OR l.address_city ILIKE sqlc.narg('search')::text
    OR u.first_name ILIKE sqlc.narg('search')::text OR u.last_name ILIKE sqlc.narg('search')::text OR u.email ILIKE sqlc.narg('search')::text
    OR EXISTS (
      SELECT 1
      FROM RAC_quote_items qi
      WHERE qi.quote_id = q.id AND qi.organization_id = q.organization_id
        AND qi.description ILIKE sqlc.narg('search')::text
    )
  ))
  AND (sqlc.narg('created_at_from')::timestamptz IS NULL OR q.created_at >= sqlc.narg('created_at_from')::timestamptz)
  AND (sqlc.narg('created_at_to')::timestamptz IS NULL OR q.created_at < sqlc.narg('created_at_to')::timestamptz)
  AND (sqlc.narg('valid_until_from')::timestamptz IS NULL OR q.valid_until >= sqlc.narg('valid_until_from')::timestamptz)
  AND (sqlc.narg('valid_until_to')::timestamptz IS NULL OR q.valid_until < sqlc.narg('valid_until_to')::timestamptz)
  AND (sqlc.narg('total_from')::bigint IS NULL OR q.total_cents >= sqlc.narg('total_from')::bigint)
  AND (sqlc.narg('total_to')::bigint IS NULL OR q.total_cents <= sqlc.narg('total_to')::bigint)
ORDER BY
  CASE WHEN sqlc.arg('sort_by')::text = 'quoteNumber' AND sqlc.arg('sort_order')::text = 'asc' THEN q.quote_number END ASC,
  CASE WHEN sqlc.arg('sort_by')::text = 'quoteNumber' AND sqlc.arg('sort_order')::text = 'desc' THEN q.quote_number END DESC,
  CASE WHEN sqlc.arg('sort_by')::text = 'status' AND sqlc.arg('sort_order')::text = 'asc' THEN q.status::text END ASC,
  CASE WHEN sqlc.arg('sort_by')::text = 'status' AND sqlc.arg('sort_order')::text = 'desc' THEN q.status::text END DESC,
  CASE WHEN sqlc.arg('sort_by')::text = 'total' AND sqlc.arg('sort_order')::text = 'asc' THEN q.total_cents END ASC,
  CASE WHEN sqlc.arg('sort_by')::text = 'total' AND sqlc.arg('sort_order')::text = 'desc' THEN q.total_cents END DESC,
  CASE WHEN sqlc.arg('sort_by')::text = 'validUntil' AND sqlc.arg('sort_order')::text = 'asc' THEN q.valid_until END ASC,
  CASE WHEN sqlc.arg('sort_by')::text = 'validUntil' AND sqlc.arg('sort_order')::text = 'desc' THEN q.valid_until END DESC,
  CASE WHEN sqlc.arg('sort_by')::text = 'customerName' AND sqlc.arg('sort_order')::text = 'asc' THEN l.consumer_last_name END ASC,
  CASE WHEN sqlc.arg('sort_by')::text = 'customerName' AND sqlc.arg('sort_order')::text = 'desc' THEN l.consumer_last_name END DESC,
  CASE WHEN sqlc.arg('sort_by')::text = 'customerPhone' AND sqlc.arg('sort_order')::text = 'asc' THEN l.consumer_phone END ASC,
  CASE WHEN sqlc.arg('sort_by')::text = 'customerPhone' AND sqlc.arg('sort_order')::text = 'desc' THEN l.consumer_phone END DESC,
  CASE WHEN sqlc.arg('sort_by')::text = 'customerAddress' AND sqlc.arg('sort_order')::text = 'asc' THEN l.address_city END ASC,
  CASE WHEN sqlc.arg('sort_by')::text = 'customerAddress' AND sqlc.arg('sort_order')::text = 'desc' THEN l.address_city END DESC,
  CASE WHEN sqlc.arg('sort_by')::text = 'createdBy' AND sqlc.arg('sort_order')::text = 'asc' THEN u.last_name END ASC,
  CASE WHEN sqlc.arg('sort_by')::text = 'createdBy' AND sqlc.arg('sort_order')::text = 'desc' THEN u.last_name END DESC,
  CASE WHEN sqlc.arg('sort_by')::text = 'createdAt' AND sqlc.arg('sort_order')::text = 'asc' THEN q.created_at END ASC,
  CASE WHEN sqlc.arg('sort_by')::text = 'createdAt' AND sqlc.arg('sort_order')::text = 'desc' THEN q.created_at END DESC,
  CASE WHEN sqlc.arg('sort_by')::text = 'updatedAt' AND sqlc.arg('sort_order')::text = 'asc' THEN q.updated_at END ASC,
  CASE WHEN sqlc.arg('sort_by')::text = 'updatedAt' AND sqlc.arg('sort_order')::text = 'desc' THEN q.updated_at END DESC,
  q.created_at DESC
LIMIT sqlc.arg('limit_count') OFFSET sqlc.arg('offset_count');

-- name: CountPendingApprovals :one
SELECT COUNT(q.id)
FROM RAC_quotes q
LEFT JOIN LATERAL (
  SELECT qar.decision
  FROM RAC_quote_ai_reviews qar
  WHERE qar.quote_id = q.id AND qar.organization_id = q.organization_id
  ORDER BY qar.created_at DESC, qar.id DESC
  LIMIT 1
) latest_review ON true
WHERE q.organization_id = $1
  AND q.status = 'Draft'
  AND (latest_review.decision IS NULL OR latest_review.decision = 'approved');

-- name: ListPendingApprovals :many
SELECT q.id, q.lead_id, q.quote_number, l.consumer_first_name, l.consumer_last_name, q.total_cents, l.lead_score, q.created_at
FROM RAC_quotes q
LEFT JOIN RAC_leads l ON l.id = q.lead_id AND l.organization_id = q.organization_id
LEFT JOIN LATERAL (
  SELECT qar.decision
  FROM RAC_quote_ai_reviews qar
  WHERE qar.quote_id = q.id AND qar.organization_id = q.organization_id
  ORDER BY qar.created_at DESC, qar.id DESC
  LIMIT 1
) latest_review ON true
WHERE q.organization_id = $1
  AND q.status = 'Draft'
  AND (latest_review.decision IS NULL OR latest_review.decision = 'approved')
ORDER BY q.updated_at DESC, q.created_at DESC
LIMIT $2 OFFSET $3;

-- name: CreateQuoteAIReview :one
INSERT INTO RAC_quote_ai_reviews (
  id, organization_id, quote_id, decision, summary,
  findings, signals, attempt_count, run_id, reviewer_name, model_name, created_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING id, organization_id, quote_id, decision, summary,
  findings, signals, attempt_count, run_id, reviewer_name, model_name, created_at;

-- name: GetLatestQuoteAIReview :one
SELECT id, organization_id, quote_id, decision, summary,
  findings, signals, attempt_count, run_id, reviewer_name, model_name, created_at
FROM RAC_quote_ai_reviews
WHERE quote_id = $1 AND organization_id = $2
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: GetLatestQuotePricingSnapshotRevision :one
SELECT COALESCE(MAX(quote_revision), 0)::int4 AS current_revision
FROM RAC_quote_pricing_snapshots
WHERE quote_id = $1 AND organization_id = $2;

-- name: CreateQuotePricingSnapshot :one
INSERT INTO RAC_quote_pricing_snapshots (
  id, quote_id, organization_id, lead_id, lead_service_id,
  service_type, postcode_raw, postcode_prefix_zip4,
  source_type, quote_revision,
  pricing_mode, discount_type, discount_value,
  material_subtotal_cents, labor_subtotal_low_cents, labor_subtotal_high_cents, extra_costs_cents,
  subtotal_cents, discount_amount_cents, tax_total_cents, total_cents,
  item_count, catalog_item_count, ad_hoc_item_count,
  structured_items, notes, price_range_text, scope_text,
  estimator_run_id, model_name, created_by_actor, created_by_user_id, created_at
)
VALUES (
  $1, $2, $3, $4, $5,
  $6, $7, $8,
  $9, $10,
  $11, $12, $13,
  $14, $15, $16, $17,
  $18, $19, $20, $21,
  $22, $23, $24,
  $25, $26, $27, $28,
  $29, $30, $31, $32, $33
)
RETURNING id, quote_id, organization_id, lead_id, lead_service_id,
  service_type, postcode_raw, postcode_prefix_zip4,
  source_type, quote_revision,
  pricing_mode, discount_type, discount_value,
  material_subtotal_cents, labor_subtotal_low_cents, labor_subtotal_high_cents, extra_costs_cents,
  subtotal_cents, discount_amount_cents, tax_total_cents, total_cents,
  item_count, catalog_item_count, ad_hoc_item_count,
  structured_items, notes, price_range_text, scope_text,
  estimator_run_id, model_name, created_by_actor, created_by_user_id, created_at;

-- name: GetLatestQuotePricingSnapshotByQuote :one
SELECT id, quote_id, organization_id, lead_id, lead_service_id,
  service_type, postcode_raw, postcode_prefix_zip4,
  source_type, quote_revision,
  pricing_mode, discount_type, discount_value,
  material_subtotal_cents, labor_subtotal_low_cents, labor_subtotal_high_cents, extra_costs_cents,
  subtotal_cents, discount_amount_cents, tax_total_cents, total_cents,
  item_count, catalog_item_count, ad_hoc_item_count,
  structured_items, notes, price_range_text, scope_text,
  estimator_run_id, model_name, created_by_actor, created_by_user_id, created_at
FROM RAC_quote_pricing_snapshots
WHERE quote_id = $1 AND organization_id = $2
ORDER BY quote_revision DESC, created_at DESC, id DESC
LIMIT 1;

-- name: GetQuotePricingSnapshotContext :one
SELECT
  st.name AS service_type,
  l.address_zip_code AS postcode_raw
FROM RAC_leads l
LEFT JOIN RAC_lead_services ls
  ON ls.id = sqlc.arg(lead_service_id)
 AND ls.lead_id = l.id
 AND ls.organization_id = l.organization_id
LEFT JOIN RAC_service_types st
  ON st.id = ls.service_type_id
 AND st.organization_id = l.organization_id
WHERE l.id = sqlc.arg(lead_id) AND l.organization_id = sqlc.arg(organization_id);

-- name: CreateQuotePricingOutcome :one
INSERT INTO RAC_quote_pricing_outcomes (
  id, quote_id, snapshot_id, organization_id, lead_id, lead_service_id,
  outcome_type, rejection_reason, accepted_total_cents, final_total_cents, estimator_run_id,
  outcome_at, metadata, created_at
)
VALUES (
  $1, $2, $3, $4, $5, $6,
  $7, $8, $9, $10, $11,
  $12, $13, $14
)
RETURNING id, quote_id, snapshot_id, organization_id, lead_id, lead_service_id,
  outcome_type, rejection_reason, accepted_total_cents, final_total_cents, estimator_run_id,
  outcome_at, metadata, created_at;

-- name: CreateQuotePricingCorrection :one
INSERT INTO RAC_quote_pricing_corrections (
  id, quote_id, snapshot_id, organization_id, quote_item_id,
  field_name, ai_value, human_value, delta_cents, delta_percentage,
  reason, ai_finding_code, estimator_run_id, created_by_user_id, created_at
)
VALUES (
  $1, $2, $3, $4, $5,
  $6, $7, $8, $9, $10,
  $11, $12, $13, $14, $15
)
RETURNING id, quote_id, snapshot_id, organization_id, quote_item_id,
  field_name, ai_value, human_value, delta_cents, delta_percentage,
  reason, ai_finding_code, estimator_run_id, created_by_user_id, created_at;

-- name: ListPricingIntelligenceAggregates :many
SELECT
  COALESCE(s.postcode_prefix_zip4, '') AS region_prefix,
  CASE
    WHEN s.total_cents < 100000 THEN 'under_1000'
    WHEN s.total_cents < 250000 THEN '1000_2500'
    WHEN s.total_cents < 500000 THEN '2500_5000'
    WHEN s.total_cents < 1000000 THEN '5000_10000'
    ELSE 'over_10000'
  END AS price_band,
  COUNT(*) FILTER (WHERE o.outcome_type IN ('accepted', 'rejected'))::int4 AS sample_count,
  COUNT(*) FILTER (WHERE o.outcome_type = 'accepted')::int4 AS accepted_count,
  COUNT(*) FILTER (WHERE o.outcome_type = 'rejected')::int4 AS rejected_count,
  COALESCE(
    ROUND(
      100.0 * COUNT(*) FILTER (WHERE o.outcome_type = 'accepted')
      / NULLIF(COUNT(*) FILTER (WHERE o.outcome_type IN ('accepted', 'rejected')), 0),
      1
    ),
    0
  )::float8 AS conversion_rate,
  COALESCE(ROUND(AVG(s.total_cents)), 0)::bigint AS average_quoted_cents,
  COALESCE(ROUND(AVG(o.final_total_cents) FILTER (WHERE o.final_total_cents IS NOT NULL)), 0)::bigint AS average_outcome_cents,
  COUNT(*) FILTER (WHERE o.final_total_cents IS NOT NULL)::int4 AS outcome_total_count
FROM RAC_quote_pricing_snapshots s
LEFT JOIN RAC_quote_pricing_outcomes o ON o.snapshot_id = s.id
WHERE s.organization_id = $1 AND s.service_type = $2
  AND ($3 = '' OR COALESCE(s.postcode_prefix_zip4, '') = $3)
GROUP BY COALESCE(s.postcode_prefix_zip4, ''), price_band
ORDER BY
  sample_count DESC,
  average_quoted_cents DESC,
  COALESCE(s.postcode_prefix_zip4, '') ASC,
  price_band ASC
LIMIT $4;

-- name: ListRecentPricingSnapshots :many
SELECT
  quote_id,
  COALESCE(postcode_prefix_zip4, '') AS region_prefix,
  CASE
    WHEN total_cents < 100000 THEN 'under_1000'
    WHEN total_cents < 250000 THEN '1000_2500'
    WHEN total_cents < 500000 THEN '2500_5000'
    WHEN total_cents < 1000000 THEN '5000_10000'
    ELSE 'over_10000'
  END AS price_band,
  source_type,
  quote_revision,
  total_cents,
  created_at
FROM RAC_quote_pricing_snapshots
WHERE organization_id = $1 AND service_type = $2
  AND ($3 = '' OR COALESCE(postcode_prefix_zip4, '') = $3)
ORDER BY
  created_at DESC,
  id DESC
LIMIT $4;

-- name: ListRecentPricingOutcomes :many
SELECT
  o.quote_id,
  COALESCE(s.postcode_prefix_zip4, '') AS region_prefix,
  CASE
    WHEN s.total_cents < 100000 THEN 'under_1000'
    WHEN s.total_cents < 250000 THEN '1000_2500'
    WHEN s.total_cents < 500000 THEN '2500_5000'
    WHEN s.total_cents < 1000000 THEN '5000_10000'
    ELSE 'over_10000'
  END AS price_band,
  o.outcome_type,
  o.final_total_cents,
  o.estimator_run_id,
  o.rejection_reason,
  o.created_at
FROM RAC_quote_pricing_outcomes o
JOIN RAC_quote_pricing_snapshots s ON s.id = o.snapshot_id
WHERE o.organization_id = $1 AND s.service_type = $2
  AND ($3 = '' OR COALESCE(s.postcode_prefix_zip4, '') = $3)
ORDER BY
  o.created_at DESC,
  o.id DESC
LIMIT $4;

-- name: ListRecentPricingCorrections :many
SELECT
  c.quote_id,
  COALESCE(s.postcode_prefix_zip4, '') AS region_prefix,
  CASE
    WHEN s.total_cents < 100000 THEN 'under_1000'
    WHEN s.total_cents < 250000 THEN '1000_2500'
    WHEN s.total_cents < 500000 THEN '2500_5000'
    WHEN s.total_cents < 1000000 THEN '5000_10000'
    ELSE 'over_10000'
  END AS price_band,
  c.field_name,
  c.delta_cents,
  c.delta_percentage,
  c.reason,
  c.ai_finding_code,
  c.estimator_run_id,
  c.created_at
FROM RAC_quote_pricing_corrections c
JOIN RAC_quote_pricing_snapshots s ON s.id = c.snapshot_id
WHERE c.organization_id = $1 AND s.service_type = $2
  AND ($3 = '' OR COALESCE(s.postcode_prefix_zip4, '') = $3)
ORDER BY
  c.created_at DESC,
  c.id DESC
LIMIT $4;

-- name: GetQuoteByPublicToken :one
SELECT id, organization_id, lead_id, lead_service_id, quote_number, status,
  pricing_mode, discount_type, discount_value,
  subtotal_cents, discount_amount_cents, tax_total_cents, total_cents,
  valid_until, notes, created_at, updated_at,
  public_token, public_token_expires_at, preview_token, preview_token_expires_at,
  viewed_at, accepted_at, rejected_at,
  rejection_reason, signature_name, signature_data, signature_ip, pdf_file_key,
  financing_disclaimer
FROM RAC_quotes WHERE public_token = $1;

-- name: GetQuoteByToken :one
SELECT id, organization_id, lead_id, lead_service_id, quote_number, status,
  pricing_mode, discount_type, discount_value,
  subtotal_cents, discount_amount_cents, tax_total_cents, total_cents,
  valid_until, notes, created_at, updated_at,
  public_token, public_token_expires_at, preview_token, preview_token_expires_at,
  viewed_at, accepted_at, rejected_at,
  rejection_reason, signature_name, signature_data, signature_ip, pdf_file_key,
  financing_disclaimer,
  CASE WHEN public_token = $1 THEN 'public' ELSE 'preview' END AS token_kind
FROM RAC_quotes
WHERE public_token = $1 OR preview_token = $1;

-- name: SetQuotePublicToken :execrows
UPDATE RAC_quotes SET public_token = $3, public_token_expires_at = $4, updated_at = $5
WHERE id = $1 AND organization_id = $2;

-- name: SetQuotePreviewToken :execrows
UPDATE RAC_quotes SET preview_token = $3, preview_token_expires_at = $4, updated_at = $5
WHERE id = $1 AND organization_id = $2;

-- name: SetQuoteViewedAt :exec
UPDATE RAC_quotes SET viewed_at = $2 WHERE id = $1 AND viewed_at IS NULL;

-- name: UpdateQuoteItemSelection :execrows
UPDATE RAC_quote_items SET is_selected = $3 WHERE id = $1 AND quote_id = $2;

-- name: UpdateQuoteTotals :exec
UPDATE RAC_quotes SET subtotal_cents = $2, discount_amount_cents = $3, tax_total_cents = $4, total_cents = $5, updated_at = $6
WHERE id = $1;

-- name: AcceptQuote :execrows
UPDATE RAC_quotes SET status = 'Accepted', accepted_at = $2, signature_name = $3, signature_data = $4, signature_ip = $5, updated_at = $2
WHERE id = $1 AND status = 'Sent';

-- name: RejectQuote :execrows
UPDATE RAC_quotes SET status = 'Rejected', rejected_at = $2, rejection_reason = $3, updated_at = $2
WHERE id = $1 AND status = 'Sent';

-- name: SetQuotePDFFileKey :exec
UPDATE RAC_quotes SET pdf_file_key = $2, updated_at = $3 WHERE id = $1;

-- name: GetQuoteItemByID :one
SELECT id, quote_id, organization_id, title, description, quantity, quantity_numeric,
  unit_price_cents, tax_rate, is_optional, is_selected, sort_order, catalog_product_id, created_at
FROM RAC_quote_items WHERE id = $1 AND quote_id = $2;

-- name: ListQuoteItemsByQuoteIDNoOrg :many
SELECT id, quote_id, organization_id, title, description, quantity, quantity_numeric,
  unit_price_cents, tax_rate, is_optional, is_selected, sort_order, catalog_product_id, created_at
FROM RAC_quote_items WHERE quote_id = $1 ORDER BY sort_order ASC;

-- name: CreateQuoteAnnotation :exec
INSERT INTO RAC_quote_annotations (id, quote_item_id, organization_id, author_type, author_id, text, is_resolved, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: ListQuoteAnnotationsByQuoteID :many
SELECT a.id, a.quote_item_id, a.organization_id, a.author_type, a.author_id, a.text, a.is_resolved, a.created_at
FROM RAC_quote_annotations a
JOIN RAC_quote_items qi ON qi.id = a.quote_item_id
WHERE qi.quote_id = $1
ORDER BY a.created_at ASC;

-- name: UpdateQuoteAnnotationText :one
UPDATE RAC_quote_annotations
SET text = $1
WHERE id = $2 AND quote_item_id = $3 AND author_type = $4
RETURNING id, quote_item_id, organization_id, author_type, author_id, text, is_resolved, created_at;

-- name: DeleteQuoteAnnotation :execrows
DELETE FROM RAC_quote_annotations WHERE id = $1 AND quote_item_id = $2 AND author_type = $3;

-- name: CreateQuoteActivity :exec
INSERT INTO RAC_quote_activity (id, quote_id, organization_id, event_type, message, metadata, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: ListQuoteActivities :many
SELECT id, quote_id, organization_id, event_type, message, metadata, created_at
FROM RAC_quote_activity
WHERE quote_id = $1 AND organization_id = $2
ORDER BY created_at DESC;

-- name: CreateGenerateQuoteJob :exec
INSERT INTO RAC_ai_quote_jobs (
  id, organization_id, user_id, lead_id, lead_service_id,
  status, step, progress_percent, error,
  quote_id, quote_number, item_count,
  started_at, updated_at, finished_at,
  feedback_rating, feedback_comment, feedback_submitted_at, cancellation_reason, viewed_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20);

-- name: GetGenerateQuoteJob :one
SELECT id, organization_id, user_id, lead_id, lead_service_id,
  status, step, progress_percent, error,
  quote_id, quote_number, item_count,
  started_at, updated_at, finished_at,
  feedback_rating, feedback_comment, feedback_submitted_at, cancellation_reason, viewed_at
FROM RAC_ai_quote_jobs
WHERE id = $1 AND organization_id = $2 AND user_id = $3;

-- name: CountGenerateQuoteJobs :one
SELECT COUNT(*)
FROM RAC_ai_quote_jobs
WHERE organization_id = $1 AND user_id = $2;

-- name: ListGenerateQuoteJobs :many
SELECT id, organization_id, user_id, lead_id, lead_service_id,
  status, step, progress_percent, error,
  quote_id, quote_number, item_count,
  started_at, updated_at, finished_at,
  feedback_rating, feedback_comment, feedback_submitted_at, cancellation_reason, viewed_at
FROM RAC_ai_quote_jobs
WHERE organization_id = $1 AND user_id = $2
ORDER BY updated_at DESC
LIMIT $3 OFFSET $4;

-- name: DeleteGenerateQuoteJob :execrows
DELETE FROM RAC_ai_quote_jobs
WHERE id = $1 AND organization_id = $2 AND user_id = $3
  AND status IN ('completed','failed','cancelled');

-- name: CancelGenerateQuoteJob :one
UPDATE RAC_ai_quote_jobs
SET status = 'cancelled',
  step = 'cancelled',
  progress_percent = CASE WHEN progress_percent > 100 THEN 100 ELSE progress_percent END,
  error = NULL,
  cancellation_reason = $6,
  updated_at = $4,
  finished_at = $5
WHERE id = $1 AND organization_id = $2 AND user_id = $3
  AND status IN ('pending', 'running')
RETURNING id, organization_id, user_id, lead_id, lead_service_id,
  status, step, progress_percent, error,
  quote_id, quote_number, item_count,
  started_at, updated_at, finished_at,
  feedback_rating, feedback_comment, feedback_submitted_at, cancellation_reason, viewed_at;

-- name: DeleteCompletedGenerateQuoteJobs :execrows
DELETE FROM RAC_ai_quote_jobs
WHERE organization_id = $1 AND user_id = $2 AND status = 'completed';

-- name: GetGenerateQuoteJobByID :one
SELECT id, organization_id, user_id, lead_id, lead_service_id,
  status, step, progress_percent, error,
  quote_id, quote_number, item_count,
  started_at, updated_at, finished_at,
  feedback_rating, feedback_comment, feedback_submitted_at, cancellation_reason, viewed_at
FROM RAC_ai_quote_jobs
WHERE id = $1;

-- name: ClaimGenerateQuoteJob :one
UPDATE RAC_ai_quote_jobs
SET status = 'running',
  step = $2,
  progress_percent = $3,
  updated_at = $4
WHERE id = $1 AND status = 'pending'
RETURNING id, organization_id, user_id, lead_id, lead_service_id,
  status, step, progress_percent, error,
  quote_id, quote_number, item_count,
  started_at, updated_at, finished_at,
  feedback_rating, feedback_comment, feedback_submitted_at, cancellation_reason, viewed_at;

-- name: UpdateGenerateQuoteJob :execrows
UPDATE RAC_ai_quote_jobs
SET status = $2,
  step = $3,
  progress_percent = $4,
  error = $5,
  quote_id = $6,
  quote_number = $7,
  item_count = $8,
  updated_at = $9,
  finished_at = $10
WHERE id = $1;

-- name: SubmitGenerateQuoteJobFeedback :one
UPDATE RAC_ai_quote_jobs
SET feedback_rating = $4,
  feedback_comment = $5,
  feedback_submitted_at = $6
WHERE id = $1 AND organization_id = $2 AND user_id = $3
  AND status IN ('completed', 'failed', 'cancelled')
RETURNING id, organization_id, user_id, lead_id, lead_service_id,
  status, step, progress_percent, error,
  quote_id, quote_number, item_count,
  started_at, updated_at, finished_at,
  feedback_rating, feedback_comment, feedback_submitted_at, cancellation_reason, viewed_at;

-- name: MarkGenerateQuoteJobViewed :one
UPDATE RAC_ai_quote_jobs
SET viewed_at = COALESCE(viewed_at, $4)
WHERE id = $1 AND organization_id = $2 AND user_id = $3
RETURNING id, organization_id, user_id, lead_id, lead_service_id,
  status, step, progress_percent, error,
  quote_id, quote_number, item_count,
  started_at, updated_at, finished_at,
  feedback_rating, feedback_comment, feedback_submitted_at, cancellation_reason, viewed_at;

-- name: DeleteFinishedGenerateQuoteJobsBefore :execrows
DELETE FROM RAC_ai_quote_jobs
WHERE
  (status = 'completed' AND finished_at IS NOT NULL AND finished_at < $1)
  OR
  (status = 'failed' AND finished_at IS NOT NULL AND finished_at < $2);

-- name: DeleteQuoteAttachmentsByQuote :exec
DELETE FROM RAC_quote_attachments WHERE quote_id = $1 AND organization_id = $2;

-- name: CreateQuoteAttachment :exec
INSERT INTO RAC_quote_attachments
  (id, quote_id, organization_id, filename, file_key, source, catalog_product_id, enabled, sort_order, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);

-- name: DeleteQuoteURLsByQuote :exec
DELETE FROM RAC_quote_urls WHERE quote_id = $1 AND organization_id = $2;

-- name: CreateQuoteURL :exec
INSERT INTO RAC_quote_urls
  (id, quote_id, organization_id, label, href, accepted, catalog_product_id, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8);

-- name: ListQuoteAttachmentsByQuoteID :many
SELECT id, quote_id, organization_id, filename, file_key, source, catalog_product_id, enabled, sort_order, created_at
FROM RAC_quote_attachments WHERE quote_id = $1 AND organization_id = $2 ORDER BY sort_order ASC;

-- name: ListQuoteURLsByQuoteID :many
SELECT id, quote_id, organization_id, label, href, accepted, catalog_product_id, created_at
FROM RAC_quote_urls WHERE quote_id = $1 AND organization_id = $2 ORDER BY created_at ASC;

-- name: ListQuoteAttachmentsByQuoteIDNoOrg :many
SELECT id, quote_id, organization_id, filename, file_key, source, catalog_product_id, enabled, sort_order, created_at
FROM RAC_quote_attachments WHERE quote_id = $1 ORDER BY sort_order ASC;

-- name: ListQuoteURLsByQuoteIDNoOrg :many
SELECT id, quote_id, organization_id, label, href, accepted, catalog_product_id, created_at
FROM RAC_quote_urls WHERE quote_id = $1 ORDER BY created_at ASC;

-- name: GetQuoteAttachmentByID :one
SELECT id, quote_id, organization_id, filename, file_key, source, catalog_product_id, enabled, sort_order, created_at
FROM RAC_quote_attachments WHERE id = $1 AND quote_id = $2 AND organization_id = $3;

-- name: GetProviderIntegration :one
SELECT organization_id, provider, is_connected, access_token, refresh_token, token_expires_at,
  administration_id, connected_by, disconnected_at, created_at, updated_at
FROM RAC_provider_integrations
WHERE organization_id = $1 AND provider = $2;

-- name: UpsertProviderIntegration :exec
INSERT INTO RAC_provider_integrations (
  organization_id, provider, is_connected, access_token, refresh_token, token_expires_at,
  administration_id, connected_by, disconnected_at, created_at, updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now(), now())
ON CONFLICT (organization_id, provider)
DO UPDATE SET
  is_connected = EXCLUDED.is_connected,
  access_token = EXCLUDED.access_token,
  refresh_token = EXCLUDED.refresh_token,
  token_expires_at = EXCLUDED.token_expires_at,
  administration_id = EXCLUDED.administration_id,
  connected_by = EXCLUDED.connected_by,
  disconnected_at = EXCLUDED.disconnected_at,
  updated_at = now();

-- name: DisconnectProviderIntegration :execrows
UPDATE RAC_provider_integrations
SET is_connected = false,
  access_token = NULL,
  refresh_token = NULL,
  token_expires_at = NULL,
  administration_id = NULL,
  disconnected_at = now(),
  updated_at = now()
WHERE organization_id = $1 AND provider = $2;

-- name: GetQuoteExport :one
SELECT id, quote_id, organization_id, provider, external_id, external_url, state, created_at, updated_at
FROM RAC_quote_exports
WHERE quote_id = $1 AND organization_id = $2 AND provider = $3;

-- name: CreateQuoteExport :exec
INSERT INTO RAC_quote_exports (
  id, quote_id, organization_id, provider, external_id, external_url, state, created_at, updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: GetQuoteStatus :one
SELECT status
FROM RAC_quotes
WHERE id = $1 AND organization_id = $2;

-- name: CreateHumanFeedback :one
INSERT INTO RAC_human_feedback (
  organization_id, quote_id, lead_service_id,
  field_changed, ai_value, human_value, delta_percentage
)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, organization_id, quote_id, lead_service_id,
  field_changed, ai_value, human_value, delta_percentage,
  context_embedding_id, applied_to_memory, created_at;
