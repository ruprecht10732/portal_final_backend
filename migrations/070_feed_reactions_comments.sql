-- 070: Feed reactions, comments and @-mentions
-- Adds social interactions to the activity feed.

-- ==============================
-- Reactions table
-- ==============================
CREATE TABLE IF NOT EXISTS RAC_feed_reactions (
  id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  event_id      TEXT        NOT NULL,          -- composite key for the feed item
  event_source  TEXT        NOT NULL,          -- category: leads, quotes, appointments, ai
  reaction_type TEXT        NOT NULL CHECK (reaction_type IN ('thumbs-up','heart','party-popper','flame')),
  user_id       UUID        NOT NULL REFERENCES RAC_users(id) ON DELETE CASCADE,
  org_id        UUID        NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),

  -- one reaction per type per user per event
  UNIQUE (event_id, event_source, reaction_type, user_id)
);

CREATE INDEX IF NOT EXISTS idx_feed_reactions_event
  ON RAC_feed_reactions (event_id, event_source);

CREATE INDEX IF NOT EXISTS idx_feed_reactions_org
  ON RAC_feed_reactions (org_id);

-- ==============================
-- Comments table
-- ==============================
CREATE TABLE IF NOT EXISTS RAC_feed_comments (
  id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  event_id      TEXT        NOT NULL,
  event_source  TEXT        NOT NULL,
  user_id       UUID        NOT NULL REFERENCES RAC_users(id) ON DELETE CASCADE,
  org_id        UUID        NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
  body          TEXT        NOT NULL CHECK (char_length(body) BETWEEN 1 AND 2000),
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_feed_comments_event
  ON RAC_feed_comments (event_id, event_source);

CREATE INDEX IF NOT EXISTS idx_feed_comments_org
  ON RAC_feed_comments (org_id);

-- ==============================
-- Comment mentions table (M2M)
-- ==============================
CREATE TABLE IF NOT EXISTS RAC_feed_comment_mentions (
  id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  comment_id    UUID        NOT NULL REFERENCES RAC_feed_comments(id) ON DELETE CASCADE,
  mentioned_user_id UUID   NOT NULL REFERENCES RAC_users(id) ON DELETE CASCADE,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),

  UNIQUE (comment_id, mentioned_user_id)
);

CREATE INDEX IF NOT EXISTS idx_feed_comment_mentions_comment
  ON RAC_feed_comment_mentions (comment_id);

CREATE INDEX IF NOT EXISTS idx_feed_comment_mentions_user
  ON RAC_feed_comment_mentions (mentioned_user_id);
