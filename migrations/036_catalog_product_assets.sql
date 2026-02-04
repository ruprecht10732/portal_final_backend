-- Catalog product assets for storing images, documents, and terms URLs
-- Files are stored in MinIO with metadata tracked in this table

CREATE TABLE catalog_product_assets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL,
    product_id UUID NOT NULL REFERENCES RAC_catalog_products(id) ON DELETE CASCADE,
    asset_type TEXT NOT NULL,
    file_key TEXT,
    file_name TEXT,
    content_type TEXT,
    size_bytes BIGINT,
    url TEXT,
    created_at TIMESTAMPTZ DEFAULT now()
);

ALTER TABLE catalog_product_assets
    ADD CONSTRAINT catalog_product_assets_asset_type_check
    CHECK (asset_type IN ('image', 'document', 'terms_url'));

ALTER TABLE catalog_product_assets
    ADD CONSTRAINT catalog_product_assets_storage_or_url_check
    CHECK (
        (file_key IS NOT NULL AND url IS NULL)
        OR (file_key IS NULL AND url IS NOT NULL)
    );

CREATE INDEX idx_catalog_product_assets_product ON catalog_product_assets(product_id);
CREATE INDEX idx_catalog_product_assets_org ON catalog_product_assets(organization_id);
CREATE INDEX idx_catalog_product_assets_product_org ON catalog_product_assets(product_id, organization_id);
CREATE INDEX idx_catalog_product_assets_product_type ON catalog_product_assets(product_id, asset_type);

COMMENT ON TABLE catalog_product_assets IS 'Stores metadata for catalog product assets (images, documents, and terms URLs)';
COMMENT ON COLUMN catalog_product_assets.file_key IS 'The object key in MinIO bucket (path including org/product prefix)';
COMMENT ON COLUMN catalog_product_assets.file_name IS 'Original filename or label';
COMMENT ON COLUMN catalog_product_assets.content_type IS 'MIME type of the file';
COMMENT ON COLUMN catalog_product_assets.size_bytes IS 'File size in bytes';
COMMENT ON COLUMN catalog_product_assets.url IS 'External URL for terms and conditions';
