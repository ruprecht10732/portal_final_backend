# ADR 0004: Unified Agent Runtime

## Status

Accepted

## Context

The leads module originally held separate fields and scheduler interfaces for each agent workspace:

- `GatekeeperScheduler` / `GatekeeperRunPayload`
- `EstimatorScheduler` / `EstimatorRunPayload`
- `DispatcherScheduler` / `DispatcherRunPayload`
- `AuditorScheduler` / `AuditVisitReportPayload` + `AuditCallLogPayload`

This design had several drawbacks:

1. **Module bloat**: `Module` needed eight individual agent fields plus separate scheduler injection methods.
2. **Payload duplication**: Five payload structs shared the same core fields (TenantID, LeadID, LeadServiceID) with minor variations.
3. **Scheduler surface area**: Four scheduler interfaces and their enqueue methods, plus a growing set of task types.
4. **Inflexible routing**: Adding a new agent workspace required changes in the module, scheduler client, scheduler worker, and handler.

## Decision

Introduce a unified runtime and task payload:

1. **`agent.Runtime`**: A single struct that holds shared dependencies and constructs workspace-specific agents on demand.
2. **`AgentTaskPayload`**: One payload struct with a `Workspace` discriminator field and optional mode-specific fields.
3. **`AgentTaskScheduler`**: One scheduler interface with `EnqueueAgentTask`.
4. **`LeadAutomationProcessor`**: One worker processor interface with `ProcessAgentTask`.
5. **Single task type**: `agent:run` replaces `leads.gatekeeper.run`, `leads.estimator.run`, `leads.dispatcher.run`, `leads.audit.visit_report`, and `leads.audit.call_log`.

### Runtime Routing

```
AgentTaskPayload.Workspace:
  "gatekeeper"  → Gatekeeper.Run
  "calculator"  → QuotingAgent (mode=estimator | quote-generator)
  "matchmaker"  → Dispatcher.Run
  "auditor"     → Auditor (AuditCallLog | AuditVisitReport)
```

### Module Simplification

Before:
```go
type Module struct {
    gatekeeper   *agent.Gatekeeper
    estimator    *agent.Estimator
    dispatcher   *agent.Dispatcher
    auditor      *agent.Auditor
    // ... 4 more fields
}
```

After:
```go
type Module struct {
    runtime *agent.Runtime
    // ... other non-agent fields
}
```

### Scheduler Unification

Before:
```go
type AutomationScheduler interface {
    GatekeeperScheduler
    EstimatorScheduler
    DispatcherScheduler
    AuditorScheduler
    // ...
}
```

After:
```go
type AutomationScheduler interface {
    scheduler.AgentTaskScheduler
    scheduler.AuditorScheduler      // deprecated, mapped to unified
    scheduler.CallLogScheduler
    scheduler.StaleLeadReEngageScheduler
}
```

## Consequences

### Positive

- **Reduced module surface area**: `Module` shrinks from ~8 agent fields to 1 `runtime` field.
- **Easier workspace addition**: Adding a new agent workspace only requires a new `case` in `Runtime.Run` and `Module.ProcessAgentTask`.
- **Consistent deduplication**: All agent tasks share the same dedup mechanism with workspace-specific TTL overrides.
- **Simpler testing**: Mock `Runtime` or `AgentTaskScheduler` instead of four separate interfaces.

### Negative

- **Payload polymorphism**: `AgentTaskPayload` carries optional fields that are only relevant to specific workspaces (e.g., `AppointmentID` for auditor). This is a mild form of "bag of fields" but is justified by the strong workspace routing contract.
- **Migration effort**: Legacy scheduler interfaces and task types are kept for backward compatibility but are deprecated.

## Migration Path

1. **New code** should use `AgentTaskScheduler.EnqueueAgentTask` with `AgentTaskPayload`.
2. **Legacy code** using deprecated interfaces still works; `Client` maps them to the unified enqueue internally.
3. **Worker** handles both old task types and the new `agent:run` via `handleAgentTask`.
4. After a transition period, deprecated interfaces and task types can be removed.

## Related

- `docs/agent-runtime-flow.md`
- `docs/agent-workspace-architecture.md`
- `docs/CHANGELOG.md`
