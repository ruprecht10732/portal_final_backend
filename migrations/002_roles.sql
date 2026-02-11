-- +goose Up
CREATE TABLE IF NOT EXISTS RAC_roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS RAC_user_roles (
    user_id UUID NOT NULL REFERENCES RAC_users(id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES RAC_roles(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, role_id)
);

-- Seed default RAC_roles
INSERT INTO RAC_roles (name)
VALUES ('admin'), ('user'), ('agent'), ('scout'), ('partner')
ON CONFLICT (name) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS RAC_user_roles;
DROP TABLE IF EXISTS RAC_roles;
