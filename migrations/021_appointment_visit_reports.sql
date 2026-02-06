-- +goose StatementBegin

CREATE TABLE IF NOT EXISTS appointment_visit_reports (
    appointment_id UUID PRIMARY KEY REFERENCES appointments(id) ON DELETE CASCADE,
    measurements TEXT,
    access_difficulty TEXT CHECK (access_difficulty IS NULL OR access_difficulty IN ('Low', 'Medium', 'High')),
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS appointment_attachments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    appointment_id UUID NOT NULL REFERENCES appointments(id) ON DELETE CASCADE,
    file_key TEXT NOT NULL,
    file_name TEXT NOT NULL,
    content_type TEXT,
    size_bytes BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_appointment_attachments_appointment_id ON appointment_attachments(appointment_id);

-- +goose StatementEnd

