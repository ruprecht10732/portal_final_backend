package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

type SearchResult struct {
	ID           uuid.UUID
	Type         string
	Title        string
	Subtitle     string
	Preview      string
	Status       string
	LinkID       string
	MatchedField string
	Score        float32
	CreatedAt    time.Time
	Total        int64
}

func (r *Repository) GlobalSearch(ctx context.Context, orgID uuid.UUID, query string, limit int, types []string) ([]SearchResult, error) {
	querySQL := `
		WITH search_query_base AS (
			SELECT
				websearch_to_tsquery('simple', rac_immutable_unaccent($2)) as q_simple,
				websearch_to_tsquery('dutch',  rac_immutable_unaccent($2)) as q_dutch,
				(
					SELECT to_tsquery('simple', string_agg(lexeme || ':*', ' & '))
					FROM (
						SELECT regexp_replace(token, '[^[:alnum:]]', '', 'g') AS lexeme
						FROM unnest(tsvector_to_array(to_tsvector('simple', rac_immutable_unaccent($2)))) AS token
					) t
					WHERE length(lexeme) >= 3
				) as q_simple_prefix,
				(
					SELECT to_tsquery('dutch', string_agg(lexeme || ':*', ' & '))
					FROM (
						SELECT regexp_replace(token, '[^[:alnum:]]', '', 'g') AS lexeme
						FROM unnest(tsvector_to_array(to_tsvector('dutch', rac_immutable_unaccent($2)))) AS token
					) t
					WHERE length(lexeme) >= 3
				) as q_dutch_prefix,
				rac_immutable_unaccent($2) as q_text
		),
		search_query AS (
			SELECT
				q_simple,
				q_dutch,
				q_simple_prefix,
				q_dutch_prefix,
				(
					q_simple || q_dutch ||
					COALESCE(q_simple_prefix, q_simple) ||
					COALESCE(q_dutch_prefix, q_dutch)
				) as q_all,
				q_text,
				(q_text || '%') as q_text_prefix
			FROM search_query_base
		),
		scoped_leads AS (
			SELECT l.id AS lead_id
			FROM RAC_leads l
			WHERE l.organization_id = $1
			UNION
			SELECT DISTINCT q.lead_id AS lead_id
			FROM RAC_quotes q
			WHERE q.organization_id = $1
			UNION
			SELECT DISTINCT a.lead_id AS lead_id
			FROM RAC_appointments a
			WHERE a.organization_id = $1 AND a.lead_id IS NOT NULL
			UNION
			SELECT DISTINCT ls.lead_id AS lead_id
			FROM RAC_lead_services ls
			WHERE ls.organization_id = $1
			UNION
			SELECT DISTINCT ln.lead_id AS lead_id
			FROM RAC_lead_notes ln
			WHERE ln.organization_id = $1
			UNION
			SELECT DISTINCT pl.lead_id AS lead_id
			FROM RAC_partner_leads pl
			WHERE pl.organization_id = $1
		),
		matching_leads AS (
			SELECT
				l.id AS lead_id,
				ln.notes_rank,
				ln.notes_preview,
				lsn.service_note_rank,
				lsn.service_note_preview,
				(
					ts_rank(
						setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(l.consumer_first_name, ''))), 'A') ||
						setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(l.consumer_last_name, ''))), 'A') ||
						setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(l.consumer_email, ''))), 'B') ||
						setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(l.consumer_phone, ''))), 'B') ||
						setweight(to_tsvector('dutch',  rac_immutable_unaccent(coalesce(l.address_city, ''))), 'C'),
						sq.q_all
					)
					+ COALESCE(ln.notes_rank * 0.30, 0)
					+ COALESCE(lsn.service_note_rank * 0.25, 0)
				) AS lead_rank
			FROM RAC_leads l
			JOIN scoped_leads sl ON sl.lead_id = l.id
			CROSS JOIN search_query sq
			LEFT JOIN LATERAL (
				SELECT
					MAX(
						ts_rank(
							to_tsvector('dutch', rac_immutable_unaccent(coalesce(ln.body, ''))),
							sq.q_all
						)
					) AS notes_rank,
					(
						SELECT ts_headline(
							'dutch',
							rac_immutable_unaccent(coalesce(ln2.body, '')),
							sq.q_all,
							'MaxWords=18, MinWords=6, ShortWord=2, StartSel=[, StopSel=]'
						)
						FROM RAC_lead_notes ln2
						WHERE ln2.organization_id = $1
							AND ln2.lead_id = l.id
							AND to_tsvector('dutch', rac_immutable_unaccent(coalesce(ln2.body, ''))) @@ sq.q_all
						ORDER BY ts_rank(
							to_tsvector('dutch', rac_immutable_unaccent(coalesce(ln2.body, ''))),
							sq.q_all
						) DESC,
						ln2.created_at DESC
						LIMIT 1
					) AS notes_preview
				FROM RAC_lead_notes ln
				WHERE ln.organization_id = $1
					AND ln.lead_id = l.id
					AND to_tsvector('dutch', rac_immutable_unaccent(coalesce(ln.body, ''))) @@ sq.q_all
			) ln ON true
			LEFT JOIN LATERAL (
				SELECT
					MAX(
						ts_rank(
							to_tsvector('dutch', rac_immutable_unaccent(coalesce(ls.consumer_note, ''))),
							sq.q_all
						)
					) AS service_note_rank,
					(
						SELECT ts_headline(
							'dutch',
							rac_immutable_unaccent(coalesce(ls2.consumer_note, '')),
							sq.q_all,
							'MaxWords=18, MinWords=6, ShortWord=2, StartSel=[, StopSel=]'
						)
						FROM RAC_lead_services ls2
						WHERE ls2.organization_id = $1
							AND ls2.lead_id = l.id
							AND ls2.consumer_note IS NOT NULL
							AND to_tsvector('dutch', rac_immutable_unaccent(coalesce(ls2.consumer_note, ''))) @@ sq.q_all
						ORDER BY ts_rank(
							to_tsvector('dutch', rac_immutable_unaccent(coalesce(ls2.consumer_note, ''))),
							sq.q_all
						) DESC,
						ls2.updated_at DESC
						LIMIT 1
					) AS service_note_preview
				FROM RAC_lead_services ls
				WHERE ls.organization_id = $1
					AND ls.lead_id = l.id
					AND ls.consumer_note IS NOT NULL
					AND to_tsvector('dutch', rac_immutable_unaccent(coalesce(ls.consumer_note, ''))) @@ sq.q_all
			) lsn ON true
			WHERE (
					setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(l.consumer_first_name, ''))), 'A') ||
					setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(l.consumer_last_name, ''))), 'A') ||
					setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(l.consumer_email, ''))), 'B') ||
					setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(l.consumer_phone, ''))), 'B') ||
					setweight(to_tsvector('dutch',  rac_immutable_unaccent(coalesce(l.address_city, ''))), 'C')
				) @@ sq.q_all
				OR ln.notes_rank IS NOT NULL
				OR lsn.service_note_rank IS NOT NULL
		),
		related_partners AS (
			SELECT
				pl.partner_id,
				MAX(ml.lead_rank) AS lead_rank
			FROM RAC_partner_leads pl
			JOIN matching_leads ml ON ml.lead_id = pl.lead_id
			WHERE pl.organization_id = $1
			GROUP BY pl.partner_id
		),
		results AS (
			-- 1) LEADS
			SELECT
				l.id,
				'lead'::text AS type,
				COALESCE(NULLIF(trim(concat_ws(' ', l.consumer_first_name, l.consumer_last_name)), ''), 'Unknown') AS title,
				concat_ws(' • ', NULLIF(l.address_city, ''), NULLIF(l.consumer_phone, '')) AS subtitle,
				COALESCE(
					NULLIF(ml.notes_preview, ''),
					NULLIF(ml.service_note_preview, ''),
					NULLIF(l.consumer_email, ''),
					NULLIF(l.consumer_phone, ''),
					NULLIF(l.address_city, ''),
					''
				) AS preview,
				CASE WHEN l.is_incomplete THEN 'Incomplete' ELSE 'Complete' END AS status,
				l.id::text AS link_id,
				CASE
					WHEN ml.notes_rank IS NOT NULL OR ml.service_note_rank IS NOT NULL THEN 'notes'
					WHEN (to_tsvector('simple', rac_immutable_unaccent(coalesce(l.consumer_first_name, ''))) || to_tsvector('simple', rac_immutable_unaccent(coalesce(l.consumer_last_name, '')))) @@ sq.q_all THEN 'name'
					WHEN to_tsvector('simple', rac_immutable_unaccent(coalesce(l.consumer_email, ''))) @@ sq.q_all THEN 'email'
					WHEN to_tsvector('simple', rac_immutable_unaccent(coalesce(l.consumer_phone, ''))) @@ sq.q_all THEN 'phone'
					WHEN to_tsvector('dutch',  rac_immutable_unaccent(coalesce(l.address_city, ''))) @@ sq.q_all THEN 'city'
					ELSE 'address'
				END AS matched_field,
				ml.lead_rank AS rank,
				COALESCE(l.created_at, l.updated_at) AS created_at
			FROM matching_leads ml
			JOIN RAC_leads l ON l.id = ml.lead_id
			, search_query sq
			WHERE l.deleted_at IS NULL

			UNION ALL

			-- 2) QUOTES
			SELECT
				q.id,
				'quote'::text AS type,
				q.quote_number AS title,
				concat_ws(' • ', NULLIF(trim(concat_ws(' ', coalesce(l.consumer_first_name, ''), coalesce(l.consumer_last_name, ''))), ''), ('Total: ' || (q.total_cents / 100.0)::text || ' EUR')) AS subtitle,
				CASE
					WHEN to_tsvector('dutch', rac_immutable_unaccent(coalesce(q.notes, ''))) @@ sq.q_all THEN ts_headline(
						'dutch',
						rac_immutable_unaccent(coalesce(q.notes, '')),
						sq.q_all,
						'MaxWords=18, MinWords=6, ShortWord=2, StartSel=[, StopSel=]'
					)
					WHEN ml.lead_id IS NOT NULL THEN NULLIF(trim(concat_ws(' ', coalesce(l.consumer_first_name, ''), coalesce(l.consumer_last_name, ''))), '')
					ELSE ''
				END AS preview,
				q.status::text AS status,
				q.id::text AS link_id,
				CASE
					WHEN ml.lead_id IS NOT NULL AND NOT (
						setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(q.quote_number, ''))), 'A') ||
						setweight(to_tsvector('dutch',  rac_immutable_unaccent(coalesce(q.notes, ''))), 'D')
					) @@ sq.q_all THEN 'lead'
					WHEN to_tsvector('simple', rac_immutable_unaccent(coalesce(q.quote_number, ''))) @@ sq.q_all THEN 'quote_number'
					WHEN to_tsvector('dutch',  rac_immutable_unaccent(coalesce(q.notes, ''))) @@ sq.q_all THEN 'notes'
					ELSE 'content'
				END AS matched_field,
				(
					CASE WHEN (
						setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(q.quote_number, ''))), 'A') ||
						setweight(to_tsvector('dutch',  rac_immutable_unaccent(coalesce(q.notes, ''))), 'D')
					) @@ sq.q_all
					THEN ts_rank(
						setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(q.quote_number, ''))), 'A') ||
						setweight(to_tsvector('dutch',  rac_immutable_unaccent(coalesce(q.notes, ''))), 'D'),
						sq.q_all
					)
					ELSE 0
					END
					+ COALESCE(ml.lead_rank * 0.20, 0)
				) AS rank,
				q.created_at
			FROM RAC_quotes q
			JOIN RAC_leads l ON l.id = q.lead_id
			LEFT JOIN matching_leads ml ON ml.lead_id = q.lead_id
			, search_query sq
			WHERE q.organization_id = $1
				AND (
					(
						setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(q.quote_number, ''))), 'A') ||
						setweight(to_tsvector('dutch',  rac_immutable_unaccent(coalesce(q.notes, ''))), 'D')
					) @@ sq.q_all
					OR ml.lead_id IS NOT NULL
				)

			UNION ALL

			-- 3) PARTNERS
			SELECT
				p.id,
				'partner'::text AS type,
				p.business_name AS title,
				concat_ws(' • ', NULLIF(p.contact_name, ''), NULLIF(p.city, '')) AS subtitle,
				concat_ws(
					' • ',
					NULLIF(p.contact_email, ''),
					NULLIF(p.contact_phone, ''),
					NULLIF(p.address_line1, ''),
					NULLIF(p.postal_code, ''),
					NULLIF(p.city, '')
				) AS preview,
				'Active'::text AS status,
				p.id::text AS link_id,
				CASE
					WHEN rp.partner_id IS NOT NULL AND NOT (
						setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(p.business_name, ''))), 'A') ||
						setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(p.contact_name, ''))), 'B') ||
						setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(p.contact_email, ''))), 'C')
					) @@ sq.q_all THEN 'lead'
					WHEN to_tsvector('simple', rac_immutable_unaccent(coalesce(p.business_name, ''))) @@ sq.q_all THEN 'business_name'
					WHEN to_tsvector('simple', rac_immutable_unaccent(coalesce(p.contact_name, ''))) @@ sq.q_all THEN 'contact_name'
					WHEN to_tsvector('simple', rac_immutable_unaccent(coalesce(p.contact_email, ''))) @@ sq.q_all THEN 'contact_email'
					ELSE 'business_name'
				END AS matched_field,
				(
					CASE WHEN (
						setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(p.business_name, ''))), 'A') ||
						setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(p.contact_name, ''))), 'B') ||
						setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(p.contact_email, ''))), 'C')
					) @@ sq.q_all
					THEN ts_rank(
						setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(p.business_name, ''))), 'A') ||
						setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(p.contact_name, ''))), 'B') ||
						setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(p.contact_email, ''))), 'C'),
						sq.q_all
					)
					ELSE 0
					END
					+ COALESCE(rp.lead_rank * 0.10, 0)
				) AS rank,
				p.created_at
			FROM RAC_partners p
			LEFT JOIN related_partners rp ON rp.partner_id = p.id
			, search_query sq
			WHERE p.organization_id = $1
				AND (
					(
						setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(p.business_name, ''))), 'A') ||
						setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(p.contact_name, ''))), 'B') ||
						setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(p.contact_email, ''))), 'C')
					) @@ sq.q_all
					OR rp.partner_id IS NOT NULL
				)

			UNION ALL

			-- 4) APPOINTMENTS
			SELECT
				a.id,
				'appointment'::text AS type,
				a.title AS title,
				concat_ws(' • ', to_char(a.start_time, 'DD-MM-YYYY HH24:MI'), coalesce(a.location, '')) AS subtitle,
				CASE
					WHEN to_tsvector('dutch', rac_immutable_unaccent(coalesce(a.description, ''))) @@ sq.q_all THEN ts_headline(
						'dutch',
						rac_immutable_unaccent(coalesce(a.description, '')),
						sq.q_all,
						'MaxWords=18, MinWords=6, ShortWord=2, StartSel=[, StopSel=]'
					)
					WHEN ml.lead_id IS NOT NULL THEN NULLIF(trim(concat_ws(' ', coalesce(al.consumer_first_name, ''), coalesce(al.consumer_last_name, ''))), '')
					ELSE coalesce(a.location, '')
				END AS preview,
				a.status::text AS status,
				a.id::text AS link_id,
				CASE
					WHEN ml.lead_id IS NOT NULL AND NOT (
						setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(a.title, ''))), 'A') ||
						setweight(to_tsvector('dutch',  rac_immutable_unaccent(coalesce(a.description, ''))), 'D') ||
						setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(a.location, ''))), 'C')
					) @@ sq.q_all THEN 'lead'
					WHEN to_tsvector('simple', rac_immutable_unaccent(coalesce(a.title, ''))) @@ sq.q_all THEN 'title'
					WHEN to_tsvector('dutch',  rac_immutable_unaccent(coalesce(a.description, ''))) @@ sq.q_all THEN 'description'
					WHEN to_tsvector('simple', rac_immutable_unaccent(coalesce(a.location, ''))) @@ sq.q_all THEN 'location'
					ELSE 'title'
				END AS matched_field,
				(
					CASE WHEN (
						setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(a.title, ''))), 'A') ||
						setweight(to_tsvector('dutch',  rac_immutable_unaccent(coalesce(a.description, ''))), 'D') ||
						setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(a.location, ''))), 'C')
					) @@ sq.q_all
					THEN ts_rank(
						setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(a.title, ''))), 'A') ||
						setweight(to_tsvector('dutch',  rac_immutable_unaccent(coalesce(a.description, ''))), 'D') ||
						setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(a.location, ''))), 'C'),
						sq.q_all
					)
					ELSE 0
					END
					+ COALESCE(ml.lead_rank * 0.15, 0)
				) AS rank,
				a.created_at
			FROM RAC_appointments a
			LEFT JOIN RAC_leads al ON al.id = a.lead_id
			LEFT JOIN matching_leads ml ON ml.lead_id = a.lead_id
			, search_query sq
			WHERE a.organization_id = $1
				AND (
					(
						setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(a.title, ''))), 'A') ||
						setweight(to_tsvector('dutch',  rac_immutable_unaccent(coalesce(a.description, ''))), 'D') ||
						setweight(to_tsvector('simple', rac_immutable_unaccent(coalesce(a.location, ''))), 'C')
					) @@ sq.q_all
					OR ml.lead_id IS NOT NULL
				)

			UNION ALL

			-- 5) CATALOG PRODUCTS
			SELECT
				cp.id,
				'catalog_product'::text AS type,
				cp.title AS title,
				concat_ws(' • ', NULLIF(cp.reference, ''), NULLIF(cp.type, '')) AS subtitle,
				COALESCE(NULLIF(cp.description, ''), '') AS preview,
				'Active'::text AS status,
				cp.id::text AS link_id,
				CASE
					WHEN rac_immutable_unaccent(coalesce(cp.title, '')) ILIKE sq.q_text_prefix THEN 'title'
					WHEN rac_immutable_unaccent(coalesce(cp.reference, '')) ILIKE sq.q_text_prefix THEN 'reference'
					ELSE 'title'
				END AS matched_field,
				CASE
					WHEN lower(rac_immutable_unaccent(coalesce(cp.title, ''))) = lower(sq.q_text) THEN 0.90
					WHEN rac_immutable_unaccent(coalesce(cp.title, '')) ILIKE sq.q_text_prefix THEN 0.80
					WHEN rac_immutable_unaccent(coalesce(cp.reference, '')) ILIKE sq.q_text_prefix THEN 0.75
					ELSE 0.70
				END AS rank,
				cp.created_at
			FROM RAC_catalog_products cp
			, search_query sq
			WHERE cp.organization_id = $1
				AND (
					rac_immutable_unaccent(coalesce(cp.title, '')) ILIKE sq.q_text_prefix
					OR rac_immutable_unaccent(coalesce(cp.reference, '')) ILIKE sq.q_text_prefix
				)

			UNION ALL

			-- 6) SERVICE TYPES
			SELECT
				st.id,
				'service_type'::text AS type,
				st.name AS title,
				st.slug AS subtitle,
				COALESCE(NULLIF(st.description, ''), '') AS preview,
				CASE WHEN st.is_active THEN 'Active' ELSE 'Inactive' END AS status,
				st.id::text AS link_id,
				CASE
					WHEN rac_immutable_unaccent(coalesce(st.name, '')) ILIKE sq.q_text_prefix THEN 'name'
					WHEN rac_immutable_unaccent(coalesce(st.slug, '')) ILIKE sq.q_text_prefix THEN 'slug'
					ELSE 'name'
				END AS matched_field,
				CASE
					WHEN lower(rac_immutable_unaccent(coalesce(st.name, ''))) = lower(sq.q_text) THEN 0.88
					WHEN rac_immutable_unaccent(coalesce(st.name, '')) ILIKE sq.q_text_prefix THEN 0.78
					WHEN rac_immutable_unaccent(coalesce(st.slug, '')) ILIKE sq.q_text_prefix THEN 0.72
					ELSE 0.68
				END AS rank,
				st.created_at
			FROM RAC_service_types st
			, search_query sq
			WHERE st.organization_id = $1
				AND (
					rac_immutable_unaccent(coalesce(st.name, '')) ILIKE sq.q_text_prefix
					OR rac_immutable_unaccent(coalesce(st.slug, '')) ILIKE sq.q_text_prefix
				)
		)
		SELECT
			id, type, title, subtitle, preview, status, link_id, matched_field, rank, created_at,
			COUNT(*) OVER() AS total
		FROM results
		WHERE ($4::text[] IS NULL OR type = ANY($4))
		ORDER BY rank DESC, created_at DESC
		LIMIT $3
	`

	var typesArg interface{}
	if len(types) > 0 {
		typesArg = types
	}

	rows, err := r.pool.Query(ctx, querySQL, orgID, query, limit, typesArg)
	if err != nil {
		return nil, fmt.Errorf("fts global search query failed: %w", err)
	}
	defer rows.Close()

	items := make([]SearchResult, 0)
	for rows.Next() {
		var item SearchResult
		if err := rows.Scan(
			&item.ID,
			&item.Type,
			&item.Title,
			&item.Subtitle,
			&item.Preview,
			&item.Status,
			&item.LinkID,
			&item.MatchedField,
			&item.Score,
			&item.CreatedAt,
			&item.Total,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return items, nil
}
