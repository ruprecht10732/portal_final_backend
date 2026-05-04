# Changelog

All notable changes to the backend agent runtime will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed
- Replaced per-agent module fields with a single `agent.Runtime` that constructs agents on demand.
- Introduced unified `AgentTaskPayload` and `AgentTaskScheduler` interface, deprecating `GatekeeperScheduler`, `EstimatorScheduler`, `DispatcherScheduler`, and `AuditorScheduler`.
- `Module` now implements `LeadAutomationProcessor.ProcessAgentTask` for unified scheduler consumption.
- `Handler` and `Orchestrator` route all agent execution through `Runtime.Run`.
- Scheduler worker handles legacy task types and the new unified `agent:run` task type via a single `handleAgentTask` handler.

## [2.0.0] - 2026-05-04

### Added
- Unified `AgentTaskPayload` that replaces five separate scheduler payloads (`GatekeeperRunPayload`, `EstimatorRunPayload`, `DispatcherRunPayload`, `AuditVisitReportPayload`, `AuditCallLogPayload`).
- `Runtime` struct in `internal/leads/agent/` that dynamically builds workspace agents on demand.
- Single `AgentTaskScheduler` interface with `EnqueueAgentTask` method.
- `Module.ProcessAgentTask` implementing `scheduler.LeadAutomationProcessor` to route unified tasks to the correct workspace.

### Changed
- `Module` now uses a single `runtime *agent.Runtime` field instead of eight individual agent fields.
- `Orchestrator` uses `Runtime` for all agent execution.
- `Handler` uses `Runtime` for both synchronous and asynchronous agent execution.
- Scheduler applies workspace-specific uniqueness TTLs while sharing the single `agent:run` task type.

### Deprecated
- `GatekeeperScheduler`, `EstimatorScheduler`, `DispatcherScheduler`, `AuditorScheduler` interfaces in `internal/scheduler/client.go`. Use `AgentTaskScheduler` instead.
- Legacy task types `leads.gatekeeper.run`, `leads.estimator.run`, `leads.dispatcher.run`, `leads.audit.visit_report`, `leads.audit.call_log`. Use `agent:run` with `AgentTaskPayload` instead.
