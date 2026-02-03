-- Photo analysis table to store AI analysis of lead photos
-- This stores the structured analysis from the PhotoAnalyzer agent

CREATE TABLE IF NOT EXISTS lead_photo_analyses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id UUID NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    service_id UUID NOT NULL REFERENCES lead_services(id) ON DELETE CASCADE,
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    
    -- Analysis content
    summary TEXT NOT NULL,
    observations JSONB NOT NULL DEFAULT '[]', -- Array of observation strings
    scope_assessment VARCHAR(20) NOT NULL CHECK (scope_assessment IN ('Small', 'Medium', 'Large', 'Unclear')),
    cost_indicators TEXT,
    safety_concerns JSONB DEFAULT '[]', -- Array of safety concern strings
    additional_info JSONB DEFAULT '[]', -- Array of additional info strings
    
    -- Metadata
    confidence_level VARCHAR(10) NOT NULL CHECK (confidence_level IN ('High', 'Medium', 'Low')),
    photo_count INTEGER NOT NULL DEFAULT 0,
    
    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for quick lookups
CREATE INDEX IF NOT EXISTS idx_photo_analyses_service_id ON lead_photo_analyses(service_id);
CREATE INDEX IF NOT EXISTS idx_photo_analyses_lead_id ON lead_photo_analyses(lead_id);
CREATE INDEX IF NOT EXISTS idx_photo_analyses_org_id ON lead_photo_analyses(org_id);

-- Comments
COMMENT ON TABLE lead_photo_analyses IS 'AI analysis of photos attached to lead services';
COMMENT ON COLUMN lead_photo_analyses.summary IS 'Concise summary of what photos show';
COMMENT ON COLUMN lead_photo_analyses.observations IS 'Array of specific observations from photos';
COMMENT ON COLUMN lead_photo_analyses.scope_assessment IS 'Assessment of work scope: Small, Medium, Large, or Unclear';
COMMENT ON COLUMN lead_photo_analyses.cost_indicators IS 'Factors that may affect pricing';
COMMENT ON COLUMN lead_photo_analyses.safety_concerns IS 'Array of safety issues found in photos';
COMMENT ON COLUMN lead_photo_analyses.additional_info IS 'Additional info or questions for the consumer';
COMMENT ON COLUMN lead_photo_analyses.confidence_level IS 'Confidence in analysis: High, Medium, Low';
