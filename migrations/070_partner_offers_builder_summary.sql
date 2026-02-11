-- +goose Up
-- Add AI-generated builder summary for partner offers (markdown)
ALTER TABLE RAC_partner_offers
  ADD COLUMN IF NOT EXISTS builder_summary TEXT;
