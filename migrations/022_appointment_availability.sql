-- +goose StatementBegin

CREATE TABLE IF NOT EXISTS appointment_availability_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES RAC_users(id) ON DELETE CASCADE,
    weekday SMALLINT NOT NULL CHECK (weekday BETWEEN 0 AND 6),
    start_time TIME NOT NULL,
    end_time TIME NOT NULL,
    timezone TEXT NOT NULL DEFAULT 'Europe/Amsterdam',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_availability_time_range CHECK (end_time > start_time)
);

CREATE INDEX IF NOT EXISTS idx_availability_rules_user_id ON appointment_availability_rules(user_id);

CREATE TABLE IF NOT EXISTS appointment_availability_overrides (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES RAC_users(id) ON DELETE CASCADE,
    date DATE NOT NULL,
    is_available BOOLEAN NOT NULL DEFAULT false,
    start_time TIME,
    end_time TIME,
    timezone TEXT NOT NULL DEFAULT 'Europe/Amsterdam',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_availability_override_time_range CHECK (end_time IS NULL OR start_time IS NULL OR end_time > start_time)
);

CREATE INDEX IF NOT EXISTS idx_availability_overrides_user_date ON appointment_availability_overrides(user_id, date);

-- +goose StatementEnd

