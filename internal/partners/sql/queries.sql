-- name: CreatePartner :one
INSERT INTO RAC_partners (
	id,
	organization_id,
	business_name,
	kvk_number,
	vat_number,
	address_line1,
	address_line2,
	house_number,
	postal_code,
	city,
	country,
	latitude,
	longitude,
	contact_name,
	contact_email,
	contact_phone,
	whatsapp_opted_in,
	created_at,
	updated_at
) VALUES (
	sqlc.arg(id)::uuid,
	sqlc.arg(organization_id)::uuid,
	sqlc.arg(business_name)::text,
	sqlc.narg(kvk_number)::text,
	sqlc.narg(vat_number)::text,
	sqlc.arg(address_line1)::text,
	sqlc.narg(address_line2)::text,
	sqlc.narg(house_number)::text,
	sqlc.arg(postal_code)::text,
	sqlc.arg(city)::text,
	sqlc.arg(country)::text,
	sqlc.narg(latitude)::float8,
	sqlc.narg(longitude)::float8,
	sqlc.arg(contact_name)::text,
	sqlc.arg(contact_email)::text,
	sqlc.arg(contact_phone)::text,
	sqlc.arg(whatsapp_opted_in)::bool,
	sqlc.arg(created_at)::timestamptz,
	sqlc.arg(updated_at)::timestamptz
)
RETURNING *;

-- name: GetPartnerByID :one
SELECT *
FROM RAC_partners
WHERE id = sqlc.arg(id)::uuid
  AND organization_id = sqlc.arg(organization_id)::uuid;

-- name: UpdatePartner :one
UPDATE RAC_partners
SET business_name = COALESCE(sqlc.narg(business_name)::text, business_name),
	kvk_number = CASE WHEN sqlc.narg(kvk_number)::text IS NULL THEN kvk_number ELSE NULLIF(sqlc.narg(kvk_number)::text, '') END,
	vat_number = CASE WHEN sqlc.narg(vat_number)::text IS NULL THEN vat_number ELSE NULLIF(sqlc.narg(vat_number)::text, '') END,
	address_line1 = COALESCE(sqlc.narg(address_line1)::text, address_line1),
	address_line2 = COALESCE(sqlc.narg(address_line2)::text, address_line2),
	house_number = COALESCE(sqlc.narg(house_number)::text, house_number),
	postal_code = COALESCE(sqlc.narg(postal_code)::text, postal_code),
	city = COALESCE(sqlc.narg(city)::text, city),
	country = COALESCE(sqlc.narg(country)::text, country),
	latitude = COALESCE(sqlc.narg(latitude)::float8, latitude),
	longitude = COALESCE(sqlc.narg(longitude)::float8, longitude),
	contact_name = COALESCE(sqlc.narg(contact_name)::text, contact_name),
	contact_email = COALESCE(sqlc.narg(contact_email)::text, contact_email),
	contact_phone = COALESCE(sqlc.narg(contact_phone)::text, contact_phone),
	whatsapp_opted_in = COALESCE(sqlc.narg(whatsapp_opted_in)::bool, whatsapp_opted_in),
	updated_at = now()
WHERE id = sqlc.arg(id)::uuid
  AND organization_id = sqlc.arg(organization_id)::uuid
RETURNING *;

-- name: DeletePartner :execrows
DELETE FROM RAC_partners
WHERE id = sqlc.arg(id)::uuid
  AND organization_id = sqlc.arg(organization_id)::uuid;

-- name: CountPartners :one
SELECT COUNT(*)::bigint
FROM RAC_partners
WHERE organization_id = sqlc.arg(organization_id)::uuid
  AND (
		sqlc.narg(search)::text IS NULL
		OR business_name ILIKE sqlc.narg(search)::text
		OR contact_name ILIKE sqlc.narg(search)::text
		OR contact_email ILIKE sqlc.narg(search)::text
		OR kvk_number ILIKE sqlc.narg(search)::text
		OR vat_number ILIKE sqlc.narg(search)::text
	  );

-- name: ListPartners :many
SELECT *
FROM RAC_partners
WHERE organization_id = sqlc.arg(organization_id)::uuid
  AND (
		sqlc.narg(search)::text IS NULL
		OR business_name ILIKE sqlc.narg(search)::text
		OR contact_name ILIKE sqlc.narg(search)::text
		OR contact_email ILIKE sqlc.narg(search)::text
		OR kvk_number ILIKE sqlc.narg(search)::text
		OR vat_number ILIKE sqlc.narg(search)::text
	  )
ORDER BY
	CASE WHEN sqlc.arg(sort_by)::text = 'businessName' AND sqlc.arg(sort_order)::text = 'asc' THEN business_name END ASC,
	CASE WHEN sqlc.arg(sort_by)::text = 'businessName' AND sqlc.arg(sort_order)::text = 'desc' THEN business_name END DESC,
	CASE WHEN sqlc.arg(sort_by)::text = 'contactName' AND sqlc.arg(sort_order)::text = 'asc' THEN contact_name END ASC,
	CASE WHEN sqlc.arg(sort_by)::text = 'contactName' AND sqlc.arg(sort_order)::text = 'desc' THEN contact_name END DESC,
	CASE WHEN sqlc.arg(sort_by)::text = 'createdAt' AND sqlc.arg(sort_order)::text = 'asc' THEN created_at END ASC,
	CASE WHEN sqlc.arg(sort_by)::text = 'createdAt' AND sqlc.arg(sort_order)::text = 'desc' THEN created_at END DESC,
	CASE WHEN sqlc.arg(sort_by)::text = 'updatedAt' AND sqlc.arg(sort_order)::text = 'asc' THEN updated_at END ASC,
	CASE WHEN sqlc.arg(sort_by)::text = 'updatedAt' AND sqlc.arg(sort_order)::text = 'desc' THEN updated_at END DESC,
	business_name ASC
LIMIT sqlc.arg(limit_count)::int
OFFSET sqlc.arg(offset_count)::int;

-- name: PartnerExists :one
SELECT EXISTS(
	SELECT 1
	FROM RAC_partners
	WHERE id = sqlc.arg(id)::uuid
	  AND organization_id = sqlc.arg(organization_id)::uuid
);

-- name: LeadExists :one
SELECT EXISTS(
	SELECT 1
	FROM RAC_leads
	WHERE id = sqlc.arg(id)::uuid
	  AND organization_id = sqlc.arg(organization_id)::uuid
);

-- name: LeadServiceExists :one
SELECT EXISTS(
	SELECT 1
	FROM RAC_lead_services
	WHERE id = sqlc.arg(id)::uuid
	  AND organization_id = sqlc.arg(organization_id)::uuid
);

-- name: GetLeadIDForService :one
SELECT lead_id
FROM RAC_lead_services
WHERE id = sqlc.arg(service_id)::uuid
  AND organization_id = sqlc.arg(organization_id)::uuid;

-- name: LinkPartnerLead :execrows
INSERT INTO RAC_partner_leads (organization_id, partner_id, lead_id)
VALUES (
	sqlc.arg(organization_id)::uuid,
	sqlc.arg(partner_id)::uuid,
	sqlc.arg(lead_id)::uuid
)
ON CONFLICT DO NOTHING;

-- name: UnlinkPartnerLead :execrows
DELETE FROM RAC_partner_leads
WHERE organization_id = sqlc.arg(organization_id)::uuid
  AND partner_id = sqlc.arg(partner_id)::uuid
  AND lead_id = sqlc.arg(lead_id)::uuid;

-- name: ListPartnerLeads :many
SELECT l.id,
	l.consumer_first_name,
	l.consumer_last_name,
	l.consumer_phone,
	l.address_street,
	COALESCE(l.address_house_number, '')::text AS house_number,
	l.address_city
FROM RAC_partner_leads pl
JOIN RAC_leads l ON l.id = pl.lead_id
WHERE pl.organization_id = sqlc.arg(organization_id)::uuid
  AND pl.partner_id = sqlc.arg(partner_id)::uuid
ORDER BY l.created_at DESC;

-- name: CreatePartnerInvite :one
INSERT INTO RAC_partner_invites (
	id,
	organization_id,
	partner_id,
	email,
	token_hash,
	expires_at,
	created_by,
	created_at,
	used_at,
	used_by,
	lead_id,
	lead_service_id
) VALUES (
	sqlc.arg(id)::uuid,
	sqlc.arg(organization_id)::uuid,
	sqlc.arg(partner_id)::uuid,
	sqlc.arg(email)::text,
	sqlc.arg(token_hash)::text,
	sqlc.arg(expires_at)::timestamptz,
	sqlc.arg(created_by)::uuid,
	sqlc.arg(created_at)::timestamptz,
	sqlc.narg(used_at)::timestamptz,
	sqlc.narg(used_by)::uuid,
	sqlc.narg(lead_id)::uuid,
	sqlc.narg(lead_service_id)::uuid
)
RETURNING *;

-- name: ListPartnerInvites :many
SELECT *
FROM RAC_partner_invites
WHERE organization_id = sqlc.arg(organization_id)::uuid
  AND partner_id = sqlc.arg(partner_id)::uuid
ORDER BY created_at DESC;

-- name: RevokePartnerInvite :one
UPDATE RAC_partner_invites
SET expires_at = now()
WHERE id = sqlc.arg(invite_id)::uuid
  AND organization_id = sqlc.arg(organization_id)::uuid
  AND used_at IS NULL
RETURNING *;

-- name: UpdatePartnerLogo :one
UPDATE RAC_partners
SET logo_file_key = sqlc.arg(file_key)::text,
	logo_file_name = sqlc.arg(file_name)::text,
	logo_content_type = sqlc.arg(content_type)::text,
	logo_size_bytes = sqlc.arg(size_bytes)::bigint,
	updated_at = now()
WHERE id = sqlc.arg(partner_id)::uuid
  AND organization_id = sqlc.arg(organization_id)::uuid
RETURNING *;

-- name: ClearPartnerLogo :one
UPDATE RAC_partners
SET logo_file_key = NULL,
	logo_file_name = NULL,
	logo_content_type = NULL,
	logo_size_bytes = NULL,
	updated_at = now()
WHERE id = sqlc.arg(partner_id)::uuid
  AND organization_id = sqlc.arg(organization_id)::uuid
RETURNING *;

-- name: CountValidServiceTypes :one
SELECT COUNT(*)::bigint
FROM RAC_service_types
WHERE organization_id = sqlc.arg(organization_id)::uuid
  AND id = ANY(sqlc.arg(service_type_ids)::uuid[]);

-- name: DeletePartnerServiceTypes :exec
DELETE FROM RAC_partner_service_types
WHERE partner_id = sqlc.arg(partner_id)::uuid;

-- name: CreatePartnerServiceType :exec
INSERT INTO RAC_partner_service_types (partner_id, service_type_id)
VALUES (
	sqlc.arg(partner_id)::uuid,
	sqlc.arg(service_type_id)::uuid
);

-- name: ListPartnerServiceTypeIDs :many
SELECT pst.service_type_id
FROM RAC_partner_service_types pst
JOIN RAC_service_types st ON st.id = pst.service_type_id
WHERE pst.partner_id = sqlc.arg(partner_id)::uuid
  AND st.organization_id = sqlc.arg(organization_id)::uuid
ORDER BY st.name ASC;

-- name: GetOrganizationName :one
SELECT name
FROM RAC_organizations
WHERE id = sqlc.arg(organization_id)::uuid;

-- name: CreatePartnerOffer :one
INSERT INTO RAC_partner_offers (
	organization_id,
	partner_id,
	lead_service_id,
	public_token,
	expires_at,
	pricing_source,
	customer_price_cents,
	vakman_price_cents,
	job_summary_short,
	builder_summary,
	status
) VALUES (
	sqlc.arg(organization_id)::uuid,
	sqlc.arg(partner_id)::uuid,
	sqlc.arg(lead_service_id)::uuid,
	sqlc.arg(public_token)::text,
	sqlc.arg(expires_at)::timestamptz,
	sqlc.arg(pricing_source)::pricing_source,
	sqlc.arg(customer_price_cents)::bigint,
	sqlc.arg(vakman_price_cents)::bigint,
	sqlc.narg(job_summary_short)::text,
	sqlc.narg(builder_summary)::text,
	'pending'
)
RETURNING id,
	organization_id,
	partner_id,
	lead_service_id,
	public_token,
	expires_at,
	pricing_source::text AS pricing_source,
	customer_price_cents,
	vakman_price_cents,
	job_summary_short,
	builder_summary,
	status::text AS status,
	accepted_at,
	rejected_at,
	rejection_reason,
	inspection_availability,
	job_availability,
	created_at,
	updated_at;

-- name: GetPartnerOfferByTokenWithContext :one
SELECT o.id,
	o.organization_id,
	o.partner_id,
	o.lead_service_id,
	o.public_token,
	o.expires_at,
	o.pricing_source::text AS pricing_source,
	o.customer_price_cents,
	o.vakman_price_cents,
	o.job_summary_short,
	o.builder_summary,
	o.status::text AS status,
	o.accepted_at,
	o.rejected_at,
	o.rejection_reason,
	o.inspection_availability,
	o.job_availability,
	o.created_at,
	o.updated_at,
	p.business_name,
	org.name,
	l.address_city,
	st.name AS service_type,
	l.lead_enrichment_postcode4,
	l.lead_enrichment_buurtcode,
	l.energy_bouwjaar,
	COALESCE(ai.urgency_level::text, '') AS urgency_level
FROM RAC_partner_offers o
JOIN RAC_partners p ON p.id = o.partner_id
JOIN RAC_organizations org ON org.id = o.organization_id
JOIN RAC_lead_services ls ON ls.id = o.lead_service_id
JOIN RAC_service_types st ON st.id = ls.service_type_id AND st.organization_id = ls.organization_id
JOIN RAC_leads l ON l.id = ls.lead_id
LEFT JOIN LATERAL (
	SELECT urgency_level
	FROM RAC_lead_ai_analysis
	WHERE lead_service_id = ls.id
	ORDER BY created_at DESC
	LIMIT 1
) ai ON true
WHERE o.public_token = sqlc.arg(public_token)::text;

-- name: GetPartnerOfferByID :one
SELECT id,
	organization_id,
	partner_id,
	lead_service_id,
	public_token,
	expires_at,
	pricing_source::text AS pricing_source,
	customer_price_cents,
	vakman_price_cents,
	job_summary_short,
	builder_summary,
	status::text AS status,
	accepted_at,
	rejected_at,
	rejection_reason,
	inspection_availability,
	job_availability,
	created_at,
	updated_at
FROM RAC_partner_offers
WHERE id = sqlc.arg(offer_id)::uuid
  AND organization_id = sqlc.arg(organization_id)::uuid;

-- name: DeletePartnerOffer :execrows
DELETE FROM RAC_partner_offers
WHERE id = sqlc.arg(offer_id)::uuid
  AND organization_id = sqlc.arg(organization_id)::uuid
  AND status = ANY(sqlc.arg(statuses)::text[]);

-- name: GetLeadServiceSummaryContext :one
SELECT ls.lead_id,
	st.name AS service_type,
	COALESCE(ai.urgency_level::text, '') AS urgency_level
FROM RAC_lead_services ls
JOIN RAC_service_types st ON st.id = ls.service_type_id AND st.organization_id = ls.organization_id
LEFT JOIN LATERAL (
	SELECT urgency_level
	FROM RAC_lead_ai_analysis
	WHERE lead_service_id = ls.id
	ORDER BY created_at DESC
	LIMIT 1
) ai ON true
WHERE ls.id = sqlc.arg(lead_service_id)::uuid
  AND ls.organization_id = sqlc.arg(organization_id)::uuid;

-- name: GetPartnerOfferByIDWithContext :one
SELECT o.id,
	o.organization_id,
	o.partner_id,
	o.lead_service_id,
	o.public_token,
	o.expires_at,
	o.pricing_source::text AS pricing_source,
	o.customer_price_cents,
	o.vakman_price_cents,
	o.job_summary_short,
	o.builder_summary,
	o.status::text AS status,
	o.accepted_at,
	o.rejected_at,
	o.rejection_reason,
	o.inspection_availability,
	o.job_availability,
	o.created_at,
	o.updated_at,
	p.business_name,
	org.name,
	l.address_city,
	st.name AS service_type,
	l.lead_enrichment_postcode4,
	l.lead_enrichment_buurtcode,
	l.energy_bouwjaar,
	COALESCE(ai.urgency_level::text, '') AS urgency_level
FROM RAC_partner_offers o
JOIN RAC_partners p ON p.id = o.partner_id
JOIN RAC_organizations org ON org.id = o.organization_id
JOIN RAC_lead_services ls ON ls.id = o.lead_service_id
JOIN RAC_service_types st ON st.id = ls.service_type_id AND st.organization_id = ls.organization_id
JOIN RAC_leads l ON l.id = ls.lead_id
LEFT JOIN LATERAL (
	SELECT urgency_level
	FROM RAC_lead_ai_analysis
	WHERE lead_service_id = ls.id
	ORDER BY created_at DESC
	LIMIT 1
) ai ON true
WHERE o.id = sqlc.arg(offer_id)::uuid
  AND o.organization_id = sqlc.arg(organization_id)::uuid;

-- name: ListLatestQuoteItemsForService :many
WITH latest_quote AS (
	SELECT id
	FROM RAC_quotes
	WHERE lead_service_id = sqlc.arg(lead_service_id)::uuid
	  AND organization_id = sqlc.arg(organization_id)::uuid
	  AND status != 'Draft'
	ORDER BY created_at DESC
	LIMIT 1
)
SELECT qi.description,
	qi.quantity::text AS quantity
FROM RAC_quote_items qi
JOIN latest_quote lq ON lq.id = qi.quote_id
WHERE qi.is_optional = FALSE OR qi.is_selected = TRUE
ORDER BY qi.sort_order ASC;

-- name: GetQuoteForPartnerOffer :one
SELECT id,
	organization_id,
	lead_id,
	lead_service_id,
	status::text AS status,
	total_cents
FROM RAC_quotes
WHERE id = sqlc.arg(quote_id)::uuid
  AND organization_id = sqlc.arg(organization_id)::uuid;

-- name: ListQuoteItemsForQuote :many
SELECT qi.description,
	qi.quantity::text AS quantity
FROM RAC_quote_items qi
WHERE qi.quote_id = sqlc.arg(quote_id)::uuid
  AND qi.organization_id = sqlc.arg(organization_id)::uuid
  AND (qi.is_optional = FALSE OR qi.is_selected = TRUE)
ORDER BY qi.sort_order ASC;

-- name: ListPartnerOffersForService :many
SELECT o.id,
	o.organization_id,
	o.partner_id,
	o.lead_service_id,
	o.public_token,
	o.expires_at,
	o.pricing_source::text AS pricing_source,
	o.customer_price_cents,
	o.vakman_price_cents,
	o.status::text AS status,
	o.accepted_at,
	o.rejected_at,
	o.rejection_reason,
	o.inspection_availability,
	o.job_availability,
	o.created_at,
	o.updated_at,
	p.business_name
FROM RAC_partner_offers o
JOIN RAC_partners p ON p.id = o.partner_id
WHERE o.lead_service_id = sqlc.arg(lead_service_id)::uuid
  AND o.organization_id = sqlc.arg(organization_id)::uuid
ORDER BY o.created_at DESC;

-- name: ListPartnerOffersByPartner :many
SELECT o.id,
	o.organization_id,
	o.partner_id,
	o.lead_service_id,
	o.public_token,
	o.expires_at,
	o.pricing_source::text AS pricing_source,
	o.customer_price_cents,
	o.vakman_price_cents,
	o.status::text AS status,
	o.accepted_at,
	o.rejected_at,
	o.rejection_reason,
	o.inspection_availability,
	o.job_availability,
	o.created_at,
	o.updated_at,
	p.business_name,
	org.name,
	l.address_city,
	st.name AS service_type,
	ls.service_type_id
FROM RAC_partner_offers o
JOIN RAC_partners p ON p.id = o.partner_id
JOIN RAC_organizations org ON org.id = o.organization_id
JOIN RAC_lead_services ls ON ls.id = o.lead_service_id
JOIN RAC_service_types st ON st.id = ls.service_type_id AND st.organization_id = ls.organization_id
JOIN RAC_leads l ON l.id = ls.lead_id
WHERE o.partner_id = sqlc.arg(partner_id)::uuid
  AND o.organization_id = sqlc.arg(organization_id)::uuid
ORDER BY o.created_at DESC;

-- name: HasActivePartnerOffer :one
SELECT EXISTS(
	SELECT 1
	FROM RAC_partner_offers
	WHERE lead_service_id = sqlc.arg(lead_service_id)::uuid
	  AND status IN ('pending', 'sent')
);

-- name: AcceptPartnerOffer :execrows
UPDATE RAC_partner_offers
SET status = 'accepted',
	accepted_at = now(),
	inspection_availability = sqlc.arg(inspection_slots)::jsonb,
	job_availability = sqlc.arg(job_slots)::jsonb,
	updated_at = now()
WHERE id = sqlc.arg(offer_id)::uuid
  AND status IN ('pending', 'sent');

-- name: RejectPartnerOffer :execrows
UPDATE RAC_partner_offers
SET status = 'rejected',
	rejected_at = now(),
	rejection_reason = sqlc.narg(rejection_reason)::text,
	updated_at = now()
WHERE id = sqlc.arg(offer_id)::uuid
  AND status IN ('pending', 'sent');

-- name: ExpirePartnerOffers :many
UPDATE RAC_partner_offers
SET status = 'expired',
	updated_at = now()
WHERE status IN ('pending', 'sent')
  AND expires_at < now()
RETURNING id, organization_id, partner_id, lead_service_id;

-- name: CountPartnerOffers :one
SELECT COUNT(*)::bigint
FROM RAC_partner_offers o
JOIN RAC_partners p ON p.id = o.partner_id
JOIN RAC_organizations org ON org.id = o.organization_id
JOIN RAC_lead_services ls ON ls.id = o.lead_service_id
JOIN RAC_service_types st ON st.id = ls.service_type_id AND st.organization_id = ls.organization_id
JOIN RAC_leads l ON l.id = ls.lead_id
WHERE o.organization_id = sqlc.arg(organization_id)::uuid
  AND (
		sqlc.narg(search)::text IS NULL
		OR p.business_name ILIKE sqlc.narg(search)::text
		OR st.name ILIKE sqlc.narg(search)::text
		OR l.address_city ILIKE sqlc.narg(search)::text
	  )
  AND (sqlc.narg(status)::text IS NULL OR o.status::text = sqlc.narg(status)::text)
  AND (sqlc.narg(partner_id)::uuid IS NULL OR o.partner_id = sqlc.narg(partner_id)::uuid)
  AND (sqlc.narg(lead_service_id)::uuid IS NULL OR o.lead_service_id = sqlc.narg(lead_service_id)::uuid)
  AND (sqlc.narg(service_type_id)::uuid IS NULL OR ls.service_type_id = sqlc.narg(service_type_id)::uuid);

-- name: ListPartnerOffers :many
SELECT o.id,
	o.organization_id,
	o.partner_id,
	o.lead_service_id,
	o.public_token,
	o.expires_at,
	o.pricing_source::text AS pricing_source,
	o.customer_price_cents,
	o.vakman_price_cents,
	o.job_summary_short,
	o.builder_summary,
	o.status::text AS status,
	o.accepted_at,
	o.rejected_at,
	o.rejection_reason,
	o.inspection_availability,
	o.job_availability,
	o.created_at,
	o.updated_at,
	p.business_name,
	org.name,
	l.address_city,
	st.name AS service_type,
	ls.service_type_id
FROM RAC_partner_offers o
JOIN RAC_partners p ON p.id = o.partner_id
JOIN RAC_organizations org ON org.id = o.organization_id
JOIN RAC_lead_services ls ON ls.id = o.lead_service_id
JOIN RAC_service_types st ON st.id = ls.service_type_id AND st.organization_id = ls.organization_id
JOIN RAC_leads l ON l.id = ls.lead_id
WHERE o.organization_id = sqlc.arg(organization_id)::uuid
  AND (
		sqlc.narg(search)::text IS NULL
		OR p.business_name ILIKE sqlc.narg(search)::text
		OR st.name ILIKE sqlc.narg(search)::text
		OR l.address_city ILIKE sqlc.narg(search)::text
	  )
  AND (sqlc.narg(status)::text IS NULL OR o.status::text = sqlc.narg(status)::text)
  AND (sqlc.narg(partner_id)::uuid IS NULL OR o.partner_id = sqlc.narg(partner_id)::uuid)
  AND (sqlc.narg(lead_service_id)::uuid IS NULL OR o.lead_service_id = sqlc.narg(lead_service_id)::uuid)
  AND (sqlc.narg(service_type_id)::uuid IS NULL OR ls.service_type_id = sqlc.narg(service_type_id)::uuid)
ORDER BY
	CASE WHEN sqlc.arg(sort_by)::text = 'createdAt' AND sqlc.arg(sort_order)::text = 'asc' THEN o.created_at END ASC,
	CASE WHEN sqlc.arg(sort_by)::text = 'createdAt' AND sqlc.arg(sort_order)::text = 'desc' THEN o.created_at END DESC,
	CASE WHEN sqlc.arg(sort_by)::text = 'expiresAt' AND sqlc.arg(sort_order)::text = 'asc' THEN o.expires_at END ASC,
	CASE WHEN sqlc.arg(sort_by)::text = 'expiresAt' AND sqlc.arg(sort_order)::text = 'desc' THEN o.expires_at END DESC,
	CASE WHEN sqlc.arg(sort_by)::text = 'status' AND sqlc.arg(sort_order)::text = 'asc' THEN o.status::text END ASC,
	CASE WHEN sqlc.arg(sort_by)::text = 'status' AND sqlc.arg(sort_order)::text = 'desc' THEN o.status::text END DESC,
	CASE WHEN sqlc.arg(sort_by)::text = 'partnerName' AND sqlc.arg(sort_order)::text = 'asc' THEN p.business_name END ASC,
	CASE WHEN sqlc.arg(sort_by)::text = 'partnerName' AND sqlc.arg(sort_order)::text = 'desc' THEN p.business_name END DESC,
	CASE WHEN sqlc.arg(sort_by)::text = 'serviceType' AND sqlc.arg(sort_order)::text = 'asc' THEN st.name END ASC,
	CASE WHEN sqlc.arg(sort_by)::text = 'serviceType' AND sqlc.arg(sort_order)::text = 'desc' THEN st.name END DESC,
	CASE WHEN sqlc.arg(sort_by)::text = 'vakmanPriceCents' AND sqlc.arg(sort_order)::text = 'asc' THEN o.vakman_price_cents END ASC,
	CASE WHEN sqlc.arg(sort_by)::text = 'vakmanPriceCents' AND sqlc.arg(sort_order)::text = 'desc' THEN o.vakman_price_cents END DESC,
	CASE WHEN sqlc.arg(sort_by)::text = 'customerPriceCents' AND sqlc.arg(sort_order)::text = 'asc' THEN o.customer_price_cents END ASC,
	CASE WHEN sqlc.arg(sort_by)::text = 'customerPriceCents' AND sqlc.arg(sort_order)::text = 'desc' THEN o.customer_price_cents END DESC,
	o.created_at DESC
LIMIT sqlc.arg(limit_count)::int
OFFSET sqlc.arg(offset_count)::int;