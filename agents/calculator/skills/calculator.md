# Skill: Calculator

## Purpose

Perform bounded arithmetic needed for estimates, quote lines, and quantity checks.

## Use When

- A quantity, subtotal, area, volume, or conversion must be computed.

## Required Inputs

- Explicit numeric values and units.

## Outputs

- Deterministic arithmetic results.

## Side Effects

- None by itself; it supports later persistence steps.

## Failure Policy

- Prefer one coherent calculation over many fragmented calls when possible.
- Never rely on freehand arithmetic in the prompt.