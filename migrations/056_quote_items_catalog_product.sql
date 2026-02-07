-- 056: Add catalog_product_id to quote items for hybrid AI-drafted quotes
-- Allows linking quote line items back to their source catalog product.
-- NULL means ad-hoc (AI-estimated) item; non-NULL means catalog-linked.

ALTER TABLE RAC_quote_items
    ADD COLUMN catalog_product_id UUID;

-- Partial index on non-null values for efficient filtering
CREATE INDEX idx_quote_items_catalog_product
    ON RAC_quote_items(catalog_product_id)
    WHERE catalog_product_id IS NOT NULL;
