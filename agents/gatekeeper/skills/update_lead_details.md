# Skill: UpdateLeadDetails

## Purpose

Correct explicit lead facts such as contact or address details when the evidence is clear.

## Use When

- Trusted runtime context clearly shows that a stored lead field is wrong or incomplete.

## Required Inputs

- Only the fields that must be corrected.
- Evidence-backed values only.

## Outputs

- Updated durable lead profile fields.

## Side Effects

- Changes the lead record used by later agents and human operators.

## Failure Policy

- Update contact or address data only when confidence is high.
- Never guess missing consumer details.