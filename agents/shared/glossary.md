# Agent Glossary

## Core Workflow Terms

### Lead
The customer-level record for a household or requester.

### Lead Service
The service-specific workstream under a lead. Most agents operate at the lead-service level.

### Pipeline Stage
The current operational phase for a lead service, such as `Triage`, `Nurturing`, `Estimation`, `Proposal`, `Fulfillment`, `Manual_Intervention`, `Lost`, or `Completed`.

### Status
The service status dimension used alongside pipeline stage, such as `New`, `Pending`, `In_Progress`, `Appointment_Scheduled`, or `Disqualified`.

### Terminal State
A combination of status and stage where automation should stop, such as `Lost`, `Completed`, or other terminal variants enforced by backend domain rules.

### Manual Intervention
An escalation state where the system intentionally stops autonomous progression and asks a human to decide the next step.

## Agent Roles

### Gatekeeper
The intake validator that determines whether the current service has enough trusted information to move beyond intake.

### Qualifier
The clarification-focused role that asks the customer for missing information when estimation is not yet safe.

### Calculator
The pricing role that scopes work, estimates price, drafts quotes, critiques draft quotes, and repairs quote defects.

### Matchmaker
The fulfillment routing role that finds matching partners and creates partner offers.

## Support Roles

### Call Logger
The post-call normalization role that converts rough call outcomes into structured updates.

### Auditor
The evidence-validation role that compares intake expectations to operational submissions like visit reports and call logs.

## Evidence Terms

### Trusted Context
Information the backend considers sufficiently reliable for agent decisions, such as repository data, confirmed visit measurements, and explicit customer preferences.

### Scope Artifact
The structured work-scope output used by Calculator flows as the source of truth for quantities and completeness.

