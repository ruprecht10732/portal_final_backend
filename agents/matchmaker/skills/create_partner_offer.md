# Skill: CreatePartnerOffer

## Purpose

Persist the actual partner offer for the selected fulfillment candidate.

## Use When

- A concrete partner should receive a formal offer.

## Required Inputs

- Chosen partner and offer context.

## Outputs

- Durable partner offer record.

## Side Effects

- Starts the partner-offer workflow.

## Failure Policy

- Do not create duplicate offers when an active flow already exists.