-- +goose Up
-- +goose StatementBegin

-- Appointments table for calendar management
-- Supports three types: lead_visit (linked to lead service), standalone (personal), blocked (time off)
CREATE TABLE appointments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES RAC_users(id) ON DELETE CASCADE,
    lead_id UUID REFERENCES RAC_leads(id) ON DELETE SET NULL,
    lead_service_id UUID REFERENCES RAC_lead_services(id) ON DELETE SET NULL,
    type TEXT NOT NULL CHECK (type IN ('lead_visit', 'standalone', 'blocked')),
    title TEXT NOT NULL,
    description TEXT,
    location TEXT,
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL DEFAULT 'scheduled' CHECK (status IN ('scheduled', 'completed', 'cancelled', 'no_show')),
    all_day BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- Constraint: lead_visit type must have lead_id and lead_service_id
    CONSTRAINT chk_lead_visit_refs CHECK (
        type != 'lead_visit' OR (lead_id IS NOT NULL AND lead_service_id IS NOT NULL)
    ),
    -- Constraint: end_time must be after start_time
    CONSTRAINT chk_time_range CHECK (end_time > start_time)
);

-- Indexes for common query patterns
CREATE INDEX idx_appointments_user_id ON appointments(user_id);
CREATE INDEX idx_appointments_lead_id ON appointments(lead_id) WHERE lead_id IS NOT NULL;
CREATE INDEX idx_appointments_start_time ON appointments(start_time);
CREATE INDEX idx_appointments_type ON appointments(type);
CREATE INDEX idx_appointments_status ON appointments(status);

-- Composite index for calendar range queries
CREATE INDEX idx_appointments_user_time_range ON appointments(user_id, start_time, end_time);

-- Index for finding appointments by lead service (for sync with lead visits)
CREATE INDEX idx_appointments_lead_service_id ON appointments(lead_service_id) WHERE lead_service_id IS NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS appointments;
-- +goose StatementEnd
