-- +goose Up
-- Enable pgvector extension and create agent memory tables for long-term
-- semantic memory persistence across agent sessions.
-- NOTE: pgvector is a HARD REQUIREMENT. The migration WILL FAIL if the
-- extension is not available. Install it before deploying.

CREATE EXTENSION IF NOT EXISTS vector;

-- Agent memory table: stores summarized session memories as vector embeddings.
CREATE TABLE IF NOT EXISTS agent_memory (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     TEXT NOT NULL,
    tenant_id   UUID NOT NULL,
    agent_name  TEXT NOT NULL,
    summary     TEXT NOT NULL,
    embedding   vector(768) NOT NULL, -- Gemini embedding-004 produces 768-dim vectors
    session_id  TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_agent_memory_user ON agent_memory (user_id, tenant_id, created_at DESC);
CREATE INDEX idx_agent_memory_agent ON agent_memory (agent_name, tenant_id, created_at DESC);

-- Vector similarity search index using ivfflat for approximate nearest neighbor search.
CREATE INDEX idx_agent_memory_embedding ON agent_memory
USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);

-- Agent memory metadata table: stores structured key-value pairs extracted
-- from conversations for deterministic state retrieval.
CREATE TABLE IF NOT EXISTS agent_memory_metadata (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    memory_id   UUID NOT NULL REFERENCES agent_memory(id) ON DELETE CASCADE,
    key         TEXT NOT NULL,
    value       TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_agent_memory_metadata_memory ON agent_memory_metadata (memory_id);

-- +goose Down
DROP TABLE IF EXISTS agent_memory_metadata;
DROP TABLE IF EXISTS agent_memory;
