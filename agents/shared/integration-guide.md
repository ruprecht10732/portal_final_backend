# Support Agent Integration Guide

This page explains how support agents feed the main lead workflow.

## Photo Analyzer

- Trigger source: image attachment uploads and explicit photo-analysis jobs.
- Primary consumers: Gatekeeper and Calculator.
- Operational effect: can defer initial Gatekeeper evaluation until visual evidence is available.
- Failure mode: photo-analysis failure creates alert context and can still wake Gatekeeper with reduced evidence.

## Call Logger

- Trigger source: submitted call summaries and post-call operations.
- Primary consumers: lead state, appointments, and Auditor call-log audits.
- Operational effect: can mutate notes, call outcomes, appointments, and lead or service fields.

## Auditor

- Trigger source: visit report submission and call-log capture.
- Primary consumers: human operators reviewing evidence quality.
- Operational effect: persists explicit findings; does not replace Gatekeeper or Calculator decisions.

## Offer Summary Generator

- Trigger source: partner-offer summary requests.
- Primary consumers: human fulfillment users and external communication surfaces.

## Reply Agents

- Trigger source: inbox and conversation assistance requests.
- Primary consumers: human operators who review or send replies.
- Operational effect: suggestions only; they do not directly change durable workflow state.

## WhatsApp Agent

- Trigger source: incoming WhatsApp messages via webhook.
- Primary consumers: end-customers who have been linked to an organization via invite code.
- Operational effect: autonomous customer support over WhatsApp with bounded write actions. It can query quotes and appointments, search leads, request or update visits, correct lead details, and record timeline notes or clarification requests when policy allows.
- Authentication: phone-number lookup in RAC_whatsapp_agent_users; unmatched users get hardcoded Dutch onboarding (zero LLM cost).
- Rate limiting: Redis sliding window, 30 messages per 5 minutes per phone number.
- Replies are persisted in the operator inbox for visibility.
- Guardrails: the agent must resolve a concrete lead, appointment slot, or appointment before mutating state and should ask one focused follow-up question when the target is ambiguous.