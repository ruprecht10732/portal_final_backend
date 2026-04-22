# Backend Gap Analysis: Enterprise Agentic Workflows
## Mapping Current Architecture Against ADK / MCP / A2A Best Practices

**Date:** 2026-04-22  
**Status:** Phase 1–3 implemented. Hardening complete. Zero legacy soft paths remain.  
**Current Stack:** Go 1.25, `google.golang.org/adk v0.4.0`, `google.golang.org/genai v1.43.0`

---

## 1. Executive Summary

All critical and high-priority gaps from the initial assessment have been **implemented and hardened**. The backend now enforces Redis sessions, pgvector memory, HITL confirmation, OpenTelemetry tracing, MCP discoverability, and on-demand skill loading with **no legacy fallback paths**.

| Tier | Area | Status | Details |
|------|------|--------|---------|
| ✅ Done | **MCP Integration** | MCP server, client, and Streamable HTTP transport implemented | `POST /api/v1/mcp` exposes domain tools via JSON-RPC |
| ✅ Done | **Persistent Session & Memory** | Redis-backed sessions mandatory; pgvector table created | Panics if Redis unavailable; `agent_memory` with `vector(768)` |
| ✅ Done | **Human-in-the-Loop (HITL)** | DB-backed provider blocks until approved/rejected | High-risk tools (`UpdatePipelineStage`, `DraftQuote`, `CreatePartnerOffer`, `CancelVisit`, `GenerateQuote`) wrapped |
| ✅ Done | **Observability** | OTel initialized; spans per agent run; token tracking live | `agent_runs.token_input/token_output` populated from `UsageMetadata` |
| ✅ Done | **A2A Interoperability** | Agent Cards endpoint live | `/api/v1/agents/capabilities` serves workspace manifests |
| ✅ Done | **Streaming & Transport** | WhatsApp streaming configurable | `StreamingModeSSE` via `WHATSAPP_AGENT_STREAMING_ENABLED` |
| ✅ Done | **Progressive Context Disclosure** | `load_skill_resource` auto-injected into every agent toolset | L3 resources fetched on demand |
| 🟡 Remaining | **Long-term Memory Pipeline** | Table exists, pipeline not wired | No `AfterAgentCallback` → summarization → embedding → `agent_memory` |
| 🟡 Remaining | **Retry & Reflect** | Plugin built, not wired to tools | No tool handler is wrapped with exponential backoff retry |
| 🟡 Remaining | **Langfuse / OTLP Export** | Custom logger exporter only | No OTLP/HTTP or Langfuse plugin integration |
| 🟢 Remaining | **MCP Toolbox (`tools.yaml`)** | Not implemented | No declarative SQL exposure for pricing/partner DB queries |
| 🟢 Remaining | **A2A gRPC Delegation** | HTTP cards only | No gRPC agent-to-agent task delegation |
| 🟢 Remaining | **RBAC Tool Filtering** | Workspace-based only | `AllowedTools` not scoped by user JWT roles |
| 🟢 Remaining | **CI/CD Trajectory Evaluation** | Harness built, not gated | `eval/` package exists; not wired to deployment pipeline |

---

## 2. What Was Implemented (Hardened — No Legacy)

### 2.1 MCP Stack (`platform/mcp/`)

| Component | File | Status |
|-----------|------|--------|
| MCP Server (JSON-RPC) | `server.go` | ✅ `tools/list`, `tools/call`, `initialize` |
| MCP Client | `client.go` | ✅ `ListTools`, `CallTool` |
| Streamable HTTP Transport | `transport.go` | ✅ Single-endpoint bidirectional; heartbeat keepalive |
| HTTP Route | `internal/http/agents/mcp.go` | ✅ Mounted at `POST /api/v1/mcp` |

**No legacy:** The server is mounted and serving. No SSE dual-endpoint fallback.

### 2.2 Redis Sessions (`platform/adk/session/`)

| Component | File | Status |
|-----------|------|--------|
| Redis `session.Service` | `redis.go` | ✅ Full ADK interface: Create, Get, List, Delete, AppendEvent |
| Factory | `factory.go` | ✅ **Panics** if `Backend != "redis"` or `RedisClient == nil` |
| Migration | All agents | ✅ 12 agents inject Redis-backed service; `InMemoryService` eliminated |

**No legacy:** `initSessionRedis()` panics if `REDIS_URL` is missing. `NewService()` panics on invalid config.

### 2.3 Token Tracking

| Component | File | Status |
|-----------|------|--------|
| UsageMetadata extraction | `platform/ai/openaicompat/model.go` | ✅ `prompt_tokens` / `completion_tokens` → `genai.GenerateContentResponseUsageMetadata` |
| Event accumulation | `internal/leads/agent/run_iterator.go` | ✅ `accumulateTokens` observer added to runner |
| Persistence | `gatekeeper.go`, `quoting_agent.go` | ✅ `token_input` / `token_output` written to `agent_runs` |

### 2.4 pgvector Memory (`migrations/189_...`)

| Component | Status |
|-----------|--------|
| `agent_memory` table | ✅ `vector(768) NOT NULL` |
| `agent_memory_metadata` table | ✅ Key-value pairs for structured state |
| `ivfflat` vector index | ✅ `vector_cosine_ops` with 100 lists |
| Fallback | ❌ **None** — migration fails loudly if pgvector missing |

### 2.5 Human-in-the-Loop (`platform/adk/confirmation/`)

| Component | File | Status |
|-----------|------|--------|
| Threshold rules | `confirmation.go` | ✅ `UpdatePipelineStage`, `DraftQuote`, `GenerateQuote`, `CreatePartnerOffer`, `CancelVisit` flagged |
| DB Provider | `db_provider.go` | ✅ Writes to `agent_approvals`; `PollDecision` blocks with ticker polling |
| Global wrapper | `global.go` | ✅ `WrapToolHandler` intercepts at handler level |
| Tool wrapping | `domain_builders.go` | ✅ 5 high-risk tool builders pass handlers through `confirmation.WrapToolHandler` |
| Wiring | `cmd/api/main.go` | ✅ `confirmation.NewDBProvider(pool)` set as global provider at startup |

**No legacy:** No auto-approve fallback. If the provider is nil, tools execute directly (expected for test/standalone), but in production the DB provider is always set.

### 2.6 OpenTelemetry (`platform/otel/`)

| Component | File | Status |
|-----------|------|--------|
| Tracer provider | `provider.go` | ✅ `InitTracerProvider` with custom `loggerExporter` |
| Span helpers | `otel.go` | ✅ `StartAgentRun`, `StartToolCall`, `StartLLMCall`, `RecordLLMTokens`, `RecordAgentRunResult` |
| Initialization | `cmd/api/main.go` | ✅ Provider created at startup, shutdown on exit |
| Agent run spans | `run_iterator.go` | ✅ Every `runPromptSession` starts a span |

### 2.7 A2A Agent Cards (`internal/http/agents/`)

| Component | Status |
|-----------|--------|
| `GET /api/v1/agents/capabilities` | ✅ Returns Agent Cards for all workspaces |
| `GET /api/v1/agents/cards/:agent` | ✅ Returns individual agent manifest |
| Module registration | ✅ Registered in `cmd/api/main.go` router |

### 2.8 WhatsApp Streaming (`internal/whatsappagent/engine/`)

| Component | Status |
|-----------|--------|
| `ModuleConfig.StreamingEnabled` | ✅ Config field added |
| `Config.WhatsAppAgentStreamingEnabled` | ✅ Env-driven |
| `agentRuntimeConfig.streamingMode` | ✅ Passed to `newAgentRuntime` |
| `StreamingModeSSE` | ✅ Used when config is true |

### 2.9 On-Demand Skills (`internal/orchestration/skills/`)

| Component | File | Status |
|-----------|------|--------|
| Resource loader | `loader.go` | ✅ Filesystem cache, path sanitization |
| Skill tools | `tool.go` | ✅ `load_skill_resource`, `list_skill_resources` |
| Global injection | `skills.go` | ✅ `InitSkillLoader` + auto-append in `BuildWorkspaceToolsets` |
| Initialization | `cmd/api/main.go` | ✅ `orchestration.MustInitSkillLoader()` at startup |

### 2.10 Parallel Context Fetching (`internal/leads/agent/`)

| Component | Status |
|-----------|--------|
| `fetchLeadContextParallel` | ✅ `errgroup`-based concurrent fetch of lead, service, notes, photo |
| Active in quoting agent | ✅ `loadAutonomousRunContext` replaced sequential calls |

---

## 3. Remaining Gaps (Post-Hardening)

### 3.1 Long-Term Memory Pipeline — 🟡 MEDIUM

**What's Missing:**
- No `AfterAgentCallback` is registered to trigger summarization when a session ends.
- No summarization service exists (would call Gemini Flash to condense session logs).
- No embedding pipeline writes to `agent_memory`.
- No `preload_memory` or `load_memory` built-in tools for agents to recall past interactions.

**Why It Matters:** Without this, agents start every conversation with zero historical context beyond the 100-message WhatsApp replay. They cannot recognize returning customers or reference past decisions.

**Recommended Path:**
1. Build `platform/adk/memory/service.go` with three methods: `Summarize(session)`, `Embed(summary)`, `Store(userID, embedding, metadata)`.
2. Register an `AfterAgentCallback` in `run_iterator.go` that calls `memoryService.AddSession(ctx, session)` on completion.
3. Add `preload_memory` as a built-in tool that queries `agent_memory` by `user_id` + cosine similarity before the first user message.

### 3.2 Retry & Reflect — 🟡 MEDIUM

**What's Missing:**
- `platform/adk/plugins/retry_reflect.go` exists as a library.
- **No tool handler is actually wrapped with it.** The `tool_safeguards.go` file has configuration hooks but no active wrapping.

**Why It Matters:** External API failures (`GetEnergyLabel`, partner timeouts) still fail the agent run immediately. No self-healing.

**Recommended Path:**
- In `internal/tools/domain_builders.go`, wrap external-API tool handlers with `plugins.WrapHandler`.
- Start with `GetEnergyLabel`, `GetISDE`, `SearchProductMaterials`, and any partner-facing tools.

### 3.3 Langfuse / OTLP Export — 🟡 MEDIUM

**What's Missing:**
- Current OTel exporter is a custom `loggerExporter` that writes to stdout.
- No OTLP/HTTP exporter sending traces to Jaeger, Tempo, or Langfuse.
- No prompt/response payload capture (just token counts and tool names).

**Why It Matters:** Production debugging requires searchable trace backends, not log grep.

**Recommended Path:**
- Add `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp` dependency.
- Replace `loggerExporter` with `otlptracehttp.New` when `OTEL_EXPORTER_OTLP_ENDPOINT` is set.
- Capture full prompt/response in span attributes for debugging (watch for PII).

### 3.4 MCP Toolbox (`tools.yaml`) — 🟢 LOW

**What's Missing:**
- No declarative `tools.yaml` for database-heavy queries.
- All SQL is handwritten in Go tool handlers.

**Why It Matters:** For rapid iteration on pricing intelligence queries, a declarative config would let data analysts update queries without recompiling.

**Recommended Path:**
- Create `agents/calculator/tools.yaml` with pre-defined SQL templates for pricing lookups.
- Load at startup and register as MCP tools.

### 3.5 A2A gRPC — 🟢 LOW

**What's Missing:**
- Agent Cards are HTTP-only JSON. No gRPC service for task delegation.
- No cryptographic signing of cards.

**Why It Matters:** Only relevant if integrating with external agent frameworks (LangGraph, BeeAI) or splitting into microservices.

**Recommended Path:**
- Defer until cross-team integration is needed.
- If needed, generate protobufs from Agent Card schema and serve on a separate gRPC port.

### 3.6 RBAC Tool Filtering — 🟢 LOW

**What's Missing:**
- `AllowedTools` comes from `SKILL.md` frontmatter, scoped by agent type.
- No user-role-based restriction (e.g., support user vs. admin user).

**Why It Matters:** In a multi-tenant admin dashboard, a support user should not be able to prompt the agent to update pipeline stages.

**Recommended Path:**
- Augment `AllowedTools` resolution with a JWT role → tool whitelist lookup.
- Pass the user's roles into `ToolDependencies` and filter at toolset build time.

### 3.7 CI/CD Trajectory Evaluation — 🟢 LOW

**What's Missing:**
- `internal/leads/agent/eval/eval.go` has the harness and golden dataset.
- No GitHub Actions / CI step runs it.
- No automated alert on trajectory drift.

**Why It Matters:** Model or prompt updates can silently break the Gatekeeper → Estimator → Dispatcher flow. Trajectory eval catches this before production.

**Recommended Path:**
- Add a CI job that spins up a test DB, seeds frozen lead fixtures, runs agents, and asserts Exact Match ≥ threshold.
- Gate merges on trajectory F1 ≥ 0.95.

---

## 4. Risk Assessment (Updated)

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| HITL blocks automation indefinitely | Medium | High | 5-minute timeout + auto-expire in `DBProvider.PollDecision` |
| Redis outage kills all agents | Low | High | Redis is already required for token blocklist; add Redis Sentinel or Cluster |
| pgvector missing in new env | Low | High | Migration fails loudly; CI should test migration on prod-like Postgres |
| ADK v0.4.0 → v0.5.0 breaking changes | Medium | High | Pin version; review release notes before upgrading |
| Streaming increases token usage | Medium | Medium | Monitor `token_input` / `token_output` per run; alert on anomaly |
| MCP tool drift (Go vs. schema) | Low | Medium | Add MCP schema validation tests |

---

## 5. Next Recommended Sprint

If prioritizing the remaining gaps:

1. **Retry & Reflect wiring** (1–2 days) — Wrap external API tools; immediate resilience gain.
2. **Memory pipeline** (3–5 days) — Summarization + embedding + `preload_memory` tool; biggest UX improvement.
3. **OTLP exporter** (1 day) — Replace logger exporter with OTLP/HTTP; production observability.
4. **CI/CD trajectory eval** (2–3 days) — Gate deployments on trajectory stability.

---

*End of Analysis*
