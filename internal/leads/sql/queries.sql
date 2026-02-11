-- Leads Domain SQL Queries
-- Add sqlc-annotated queries here as you migrate raw SQL from repository files.

-- name: GetLeadByID :one
SELECT * FROM rac_leads WHERE id = $1 AND organization_id = $2;
