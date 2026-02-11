-- +goose Up
-- Smart Feed: schema prep for future roadmap features + performance indexes
-- Migration: 069_smart_feed_prep.sql

-- =============================================
-- 1. Performance indexes for clustering window functions
-- =============================================

-- Composite index to support the 15-minute clustering window function
CREATE INDEX IF NOT EXISTS idx_lead_activity_cluster
  ON RAC_lead_activity(organization_id, lead_id, action, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_quote_activity_cluster
  ON RAC_quote_activity(organization_id, event_type, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_timeline_events_cluster
  ON lead_timeline_events(organization_id, lead_id, event_type, created_at DESC);

-- General performance: support the main UNION ALL WHERE clause
CREATE INDEX IF NOT EXISTS idx_lead_activity_org_created
  ON RAC_lead_activity(organization_id, created_at DESC);


-- =============================================
-- 2. Read State — "New" line separator (Future)
-- =============================================

-- Track when each user last viewed the activity feed.
-- Used to inject a "New updates" separator in the feed response.
CREATE TABLE IF NOT EXISTS RAC_feed_read_state (
  user_id UUID NOT NULL REFERENCES RAC_users(id) ON DELETE CASCADE,
  organization_id UUID NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
  last_feed_viewed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  feed_mode TEXT NOT NULL DEFAULT 'all'
    CHECK (feed_mode IN ('all', 'high_signal')),
  PRIMARY KEY (user_id, organization_id)
);


-- =============================================
-- 3. Threaded Comments / Reactions on events (Future)
-- =============================================

-- Allow notes to be threaded as replies to specific feed events.
-- parent_event_type identifies which source table the event came from.
ALTER TABLE RAC_lead_notes
  ADD COLUMN IF NOT EXISTS parent_event_id UUID,
  ADD COLUMN IF NOT EXISTS parent_event_type TEXT CHECK (
    parent_event_type IS NULL OR parent_event_type IN (
      'lead_activity', 'quote_activity', 'timeline_event', 'appointment'
    )
  );

CREATE INDEX IF NOT EXISTS idx_lead_notes_parent_event
  ON RAC_lead_notes(parent_event_id) WHERE parent_event_id IS NOT NULL;


-- =============================================
-- 4. Pinned / Sticky Alert Cards (Future)
-- =============================================

-- Force-pinned alert cards that float to the top of the feed
-- until resolved or dismissed. Supports "manual_intervention" and
-- "urgent_analysis" alert types.
CREATE TABLE IF NOT EXISTS RAC_feed_pinned_alerts (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id UUID NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
  lead_id UUID NOT NULL REFERENCES RAC_leads(id) ON DELETE CASCADE,
  alert_type TEXT NOT NULL,
  resolved_at TIMESTAMPTZ,
  dismissed_by UUID REFERENCES RAC_users(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_feed_pinned_alerts_org
  ON RAC_feed_pinned_alerts(organization_id) WHERE resolved_at IS NULL;
