-- +goose Up
-- Lead service attachments for storing files (photos, documents, videos) per service inquiry
-- Files are stored in MinIO with metadata tracked in this table

CREATE TABLE lead_service_attachments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_service_id UUID NOT NULL REFERENCES RAC_lead_services(id) ON DELETE CASCADE,
    organization_id UUID NOT NULL,
    file_key TEXT NOT NULL,
    file_name TEXT NOT NULL,
    content_type TEXT,
    size_bytes BIGINT,
    uploaded_by UUID REFERENCES RAC_users(id),
    created_at TIMESTAMPTZ DEFAULT now()
);

-- Index for listing attachments by service
CREATE INDEX idx_lead_service_attachments_service ON lead_service_attachments(lead_service_id);

-- Index for tenant isolation queries
CREATE INDEX idx_lead_service_attachments_org ON lead_service_attachments(organization_id);

-- Composite index for common query pattern
CREATE INDEX idx_lead_service_attachments_service_org ON lead_service_attachments(lead_service_id, organization_id);

COMMENT ON TABLE lead_service_attachments IS 'Stores metadata for files uploaded to MinIO for lead services';
COMMENT ON COLUMN lead_service_attachments.file_key IS 'The object key in MinIO bucket (path including org/lead/service prefix)';
COMMENT ON COLUMN lead_service_attachments.file_name IS 'Original filename as uploaded by user';
COMMENT ON COLUMN lead_service_attachments.content_type IS 'MIME type of the file';
COMMENT ON COLUMN lead_service_attachments.size_bytes IS 'File size in bytes';
COMMENT ON COLUMN lead_service_attachments.uploaded_by IS 'User who uploaded the file';
