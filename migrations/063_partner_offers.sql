-- +goose Up
-- Partner Offers: Tracks job offers to vakman partners, pricing, and commitment lifecycle.
-- Supports sequential offer model (one active offer at a time per lead service).

CREATE TYPE offer_status AS ENUM ('pending', 'sent', 'accepted', 'rejected', 'expired');
CREATE TYPE pricing_source AS ENUM ('quote', 'estimate');

CREATE TABLE IF NOT EXISTS RAC_partner_offers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
    partner_id UUID NOT NULL REFERENCES RAC_partners(id) ON DELETE CASCADE,
    lead_service_id UUID NOT NULL REFERENCES RAC_lead_services(id) ON DELETE CASCADE,

    -- Security: unguessable public token for partner-facing URLs
    public_token TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,

    -- Economics (immutable snapshot at creation time)
    pricing_source pricing_source NOT NULL,
    customer_price_cents BIGINT NOT NULL CHECK (customer_price_cents >= 0),
    vakman_price_cents BIGINT NOT NULL CHECK (vakman_price_cents >= 0),

    -- Lifecycle
    status offer_status NOT NULL DEFAULT 'pending',

    -- Commitment data (filled on acceptance)
    accepted_at TIMESTAMPTZ,
    rejected_at TIMESTAMPTZ,
    rejection_reason TEXT,
    inspection_availability JSONB,
    job_availability JSONB,

    -- Audit
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Lookup by public token (partner-facing URL)
CREATE INDEX idx_partner_offers_token ON RAC_partner_offers(public_token);

-- List offers for a given lead service
CREATE INDEX idx_partner_offers_service ON RAC_partner_offers(lead_service_id);

-- List offers for a given partner
CREATE INDEX idx_partner_offers_partner ON RAC_partner_offers(partner_id);

-- Background expiration job
CREATE INDEX idx_partner_offers_expiry ON RAC_partner_offers(status, expires_at)
    WHERE status IN ('pending', 'sent');

-- Exclusivity: only ONE accepted offer per lead_service_id
CREATE UNIQUE INDEX idx_partner_offers_exclusive_acceptance
    ON RAC_partner_offers(lead_service_id)
    WHERE status = 'accepted';

-- +goose Down
DROP TABLE IF EXISTS RAC_partner_offers;
DROP TYPE IF EXISTS pricing_source;
DROP TYPE IF EXISTS offer_status;
