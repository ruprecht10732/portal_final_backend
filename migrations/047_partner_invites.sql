-- +goose Up
-- Create partner invites tracking table for dispatcher workflow
CREATE TABLE partner_invites (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_service_id UUID NOT NULL REFERENCES RAC_lead_services(id) ON DELETE CASCADE,
    partner_id UUID NOT NULL REFERENCES RAC_partners(id) ON DELETE CASCADE,
    organization_id UUID NOT NULL,
    
    -- Invite status tracking
    status TEXT NOT NULL CHECK (status IN ('pending', 'accepted', 'rejected', 'expired')),
    
    -- Timestamps
    invited_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    responded_at TIMESTAMPTZ,
    
    -- Additional metadata
    distance_km NUMERIC(10, 2), -- Distance from lead to partner in km
    invite_metadata JSONB DEFAULT '{}', -- Additional context (price range, urgency, etc.)
    
    CONSTRAINT fk_organization FOREIGN KEY (organization_id) REFERENCES RAC_organizations(id) ON DELETE CASCADE
);

-- Indexes for efficient querying
CREATE INDEX idx_invites_by_service ON partner_invites(lead_service_id, status);
CREATE INDEX idx_invites_by_partner ON partner_invites(partner_id, status, invited_at DESC);
CREATE INDEX idx_invites_by_org ON partner_invites(organization_id, invited_at DESC);

-- Prevent duplicate invites for same partner/service
CREATE UNIQUE INDEX idx_unique_partner_service_invite ON partner_invites(lead_service_id, partner_id);

