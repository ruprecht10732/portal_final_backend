-- +goose Up
-- Human-in-the-Loop approval table for high-stakes agent tool executions.

CREATE TABLE IF NOT EXISTS agent_approvals (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_name      TEXT NOT NULL,
    tool_name       TEXT NOT NULL,
    arguments_json  JSONB,
    reason          TEXT NOT NULL DEFAULT '',
    requested_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at      TIMESTAMPTZ,
    decision        TEXT NOT NULL DEFAULT 'pending', -- pending, approved, rejected, expired
    decided_at      TIMESTAMPTZ,
    decided_by      TEXT,
    lead_id         UUID,
    service_id      UUID,
    tenant_id       UUID NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_agent_approvals_pending ON agent_approvals (decision, tenant_id, requested_at DESC);
CREATE INDEX idx_agent_approvals_lead ON agent_approvals (lead_id, tenant_id);
CREATE INDEX idx_agent_approvals_service ON agent_approvals (service_id, tenant_id);
CREATE INDEX idx_agent_approvals_tool ON agent_approvals (tool_name, tenant_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS agent_approvals;
