# Unified Agent Runtime Migration Guide

This guide helps migrate code from the legacy per-agent scheduler interfaces to the v2.0 unified `AgentTaskScheduler` and `AgentTaskPayload`.

## Quick Reference

| Legacy Interface | Legacy Method | New Interface | New Method |
|-----------------|---------------|---------------|------------|
| `GatekeeperScheduler` | `EnqueueGatekeeperRun` | `AgentTaskScheduler` | `EnqueueAgentTask` |
| `EstimatorScheduler` | `EnqueueEstimatorRun` | `AgentTaskScheduler` | `EnqueueAgentTask` |
| `DispatcherScheduler` | `EnqueueDispatcherRun` | `AgentTaskScheduler` | `EnqueueAgentTask` |
| `AuditorScheduler` | `EnqueueAuditVisitReport` / `EnqueueAuditCallLog` | `AgentTaskScheduler` | `EnqueueAgentTask` |

## Before / After Examples

### Enqueueing a Gatekeeper Run

**Before:**
```go
err := scheduler.EnqueueGatekeeperRun(ctx, scheduler.GatekeeperRunPayload{
    TenantID:      tenantID.String(),
    LeadID:        leadID.String(),
    LeadServiceID: serviceID.String(),
})
```

**After:**
```go
err := scheduler.EnqueueAgentTask(ctx, scheduler.AgentTaskPayload{
    Workspace:     "gatekeeper",
    TenantID:      tenantID.String(),
    LeadID:        leadID.String(),
    LeadServiceID: serviceID.String(),
})
```

### Enqueueing an Estimator Run

**Before:**
```go
err := scheduler.EnqueueEstimatorRun(ctx, scheduler.EstimatorRunPayload{
    TenantID:      tenantID.String(),
    LeadID:        leadID.String(),
    LeadServiceID: serviceID.String(),
    Force:         true,
})
```

**After:**
```go
err := scheduler.EnqueueAgentTask(ctx, scheduler.AgentTaskPayload{
    Workspace:     "calculator",
    Mode:          "estimator",
    TenantID:      tenantID.String(),
    LeadID:        leadID.String(),
    LeadServiceID: serviceID.String(),
    Force:         true,
})
```

### Enqueueing a Dispatcher Run

**Before:**
```go
err := scheduler.EnqueueDispatcherRun(ctx, scheduler.DispatcherRunPayload{
    TenantID:      tenantID.String(),
    LeadID:        leadID.String(),
    LeadServiceID: serviceID.String(),
})
```

**After:**
```go
err := scheduler.EnqueueAgentTask(ctx, scheduler.AgentTaskPayload{
    Workspace:     "matchmaker",
    TenantID:      tenantID.String(),
    LeadID:        leadID.String(),
    LeadServiceID: serviceID.String(),
})
```

### Enqueueing an Auditor Visit Report

**Before:**
```go
err := scheduler.EnqueueAuditVisitReport(ctx, scheduler.AuditVisitReportPayload{
    TenantID:      tenantID.String(),
    LeadID:        leadID.String(),
    LeadServiceID: serviceID.String(),
    AppointmentID: appointmentID.String(),
})
```

**After:**
```go
err := scheduler.EnqueueAgentTask(ctx, scheduler.AgentTaskPayload{
    Workspace:     "auditor",
    TenantID:      tenantID.String(),
    LeadID:        leadID.String(),
    LeadServiceID: serviceID.String(),
    AppointmentID: appointmentID.String(),
})
```

### Enqueueing an Auditor Call Log

**Before:**
```go
err := scheduler.EnqueueAuditCallLog(ctx, scheduler.AuditCallLogPayload{
    TenantID:      tenantID.String(),
    LeadID:        leadID.String(),
    LeadServiceID: serviceID.String(),
})
```

**After:**
```go
err := scheduler.EnqueueAgentTask(ctx, scheduler.AgentTaskPayload{
    Workspace:     "auditor",
    TenantID:      tenantID.String(),
    LeadID:        leadID.String(),
    LeadServiceID: serviceID.String(),
})
```

## Module Integration

If your code previously interacted with agent fields directly on `leads.Module`, use `Runtime` instead:

**Before:**
```go
module.gatekeeper.Run(ctx, leadID, serviceID, tenantID)
```

**After:**
```go
module.runtime.Run(ctx, agent.AgentTaskPayload{
    Workspace: "gatekeeper",
    LeadID:    leadID,
    ServiceID: serviceID,
    TenantID:  tenantID,
})
```

Or use the module's convenience methods:
```go
module.ProcessGatekeeperRun(ctx, leadID, serviceID, tenantID)
module.ProcessEstimatorRun(ctx, leadID, serviceID, tenantID, force)
module.ProcessDispatcherRun(ctx, leadID, serviceID, tenantID)
module.ProcessAuditVisitReportJob(ctx, leadID, serviceID, tenantID, appointmentID)
module.ProcessAuditCallLogJob(ctx, leadID, serviceID, tenantID)
```

## Scheduler Worker

The scheduler worker now routes all legacy and new agent task types through a single handler:

```go
mux.HandleFunc(TaskRunGatekeeper, w.handleAgentTask)
mux.HandleFunc(TaskRunEstimator, w.handleAgentTask)
mux.HandleFunc(TaskRunDispatcher, w.handleAgentTask)
mux.HandleFunc(TaskAuditVisitReport, w.handleAgentTask)
mux.HandleFunc(TaskAuditCallLog, w.handleAgentTask)
mux.HandleFunc(TaskAgentRun, w.handleAgentTask)
```

All paths converge on:
```go
func (w *Worker) handleAgentTask(ctx context.Context, task *asynq.Task) error {
    payload, err := ParseAgentTaskPayload(task)
    // ...
    return w.leadsAI.ProcessAgentTask(ctx, payload)
}
```

## Deprecated APIs

The following are deprecated and will be removed in a future release:

- `scheduler.GatekeeperScheduler`
- `scheduler.EstimatorScheduler`
- `scheduler.DispatcherScheduler`
- `scheduler.AuditorScheduler`
- `scheduler.GatekeeperRunPayload`
- `scheduler.EstimatorRunPayload`
- `scheduler.DispatcherRunPayload`
- `scheduler.AuditVisitReportPayload`
- `scheduler.AuditCallLogPayload`
- Task types: `leads.gatekeeper.run`, `leads.estimator.run`, `leads.dispatcher.run`, `leads.audit.visit_report`, `leads.audit.call_log`

## Related

- `docs/adr/0004-unified-agent-runtime.md`
- `docs/agent-runtime-flow.md`
- `docs/agent-workspace-architecture.md`
