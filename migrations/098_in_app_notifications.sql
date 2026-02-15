-- +goose Up
CREATE TABLE IF NOT EXISTS RAC_in_app_notifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES RAC_organizations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES RAC_users(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    content TEXT NOT NULL,
    resource_id UUID,
    resource_type TEXT,
    category TEXT NOT NULL DEFAULT 'info',
    is_read BOOLEAN NOT NULL DEFAULT FALSE,
    read_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
        CONSTRAINT rac_in_app_notifications_org_member_fkey
            FOREIGN KEY (organization_id, user_id)
            REFERENCES RAC_organization_members(organization_id, user_id)
            ON DELETE CASCADE,
    CONSTRAINT rac_in_app_notifications_category_chk CHECK (category IN ('info', 'success', 'warning', 'error'))
);

CREATE INDEX IF NOT EXISTS idx_notifications_user_feed
ON RAC_in_app_notifications (user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_notifications_user_unread
ON RAC_in_app_notifications (user_id)
WHERE is_read = FALSE;

-- +goose Down
DROP INDEX IF EXISTS idx_notifications_user_unread;
DROP INDEX IF EXISTS idx_notifications_user_feed;
DROP TABLE IF EXISTS RAC_in_app_notifications;