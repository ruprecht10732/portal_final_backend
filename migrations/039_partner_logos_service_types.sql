ALTER TABLE RAC_partners
  ADD COLUMN logo_file_key text,
  ADD COLUMN logo_file_name text,
  ADD COLUMN logo_content_type text,
  ADD COLUMN logo_size_bytes bigint;

CREATE TABLE RAC_partner_service_types (
  partner_id uuid NOT NULL REFERENCES RAC_partners(id) ON DELETE CASCADE,
  service_type_id uuid NOT NULL REFERENCES RAC_service_types(id) ON DELETE RESTRICT,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (partner_id, service_type_id)
);

CREATE INDEX idx_partner_service_types_service_type_id
  ON RAC_partner_service_types(service_type_id);
