-- Agent run and tool call observability tables.

CREATE TABLE IF NOT EXISTS agent_runs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lead_id         UUID NOT NULL,
    service_id      UUID NOT NULL,
    tenant_id       UUID NOT NULL,
    agent_name      TEXT NOT NULL,
    run_id          TEXT NOT NULL,
    session_label   TEXT NOT NULL DEFAULT '',
    model_used      TEXT NOT NULL DEFAULT '',
    reasoning_mode  TEXT NOT NULL DEFAULT '',
    started_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at     TIMESTAMPTZ,
    duration_ms     INTEGER,
    tool_call_count INTEGER NOT NULL DEFAULT 0,
    token_input     INTEGER NOT NULL DEFAULT 0,
    token_output    INTEGER NOT NULL DEFAULT 0,
    outcome         TEXT NOT NULL DEFAULT 'unknown',
    outcome_detail  TEXT NOT NULL DEFAULT '',
    cycle_count     INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_agent_runs_service ON agent_runs (service_id, tenant_id);
CREATE INDEX idx_agent_runs_lead    ON agent_runs (lead_id, tenant_id);
CREATE INDEX idx_agent_runs_agent   ON agent_runs (agent_name, tenant_id, created_at DESC);
CREATE INDEX idx_agent_runs_outcome ON agent_runs (outcome, tenant_id, created_at DESC);

CREATE TABLE IF NOT EXISTS agent_tool_calls (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_run_id    UUID NOT NULL REFERENCES agent_runs(id) ON DELETE CASCADE,
    sequence_num    INTEGER NOT NULL DEFAULT 0,
    tool_name       TEXT NOT NULL,
    arguments_json  JSONB,
    response_json   JSONB,
    has_error       BOOLEAN NOT NULL DEFAULT false,
    error_message   TEXT NOT NULL DEFAULT '',
    duration_ms     INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_agent_tool_calls_run ON agent_tool_calls (agent_run_id);
CREATE INDEX idx_agent_tool_calls_name ON agent_tool_calls (tool_name, created_at DESC);
