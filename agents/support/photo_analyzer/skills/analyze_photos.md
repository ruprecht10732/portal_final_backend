# Skill: AnalyzePhotos

## Purpose

Use the image-analysis runtime to extract visual evidence, discrepancies, and measurement-relevant findings.

## Use When

- Image attachments exist and visual evidence can change intake or scope decisions.

## Required Inputs

- Uploaded images.
- Optional OCR assistance and preprocessing metadata.
- Service type and intake context when available.

## Outputs

- Structured photo analysis.
- Onsite-measurement flags where necessary.

## Side Effects

- Can wake Gatekeeper and influence Calculator scoping.

## Failure Policy

- Keep observations grounded in the actual images and trusted OCR assistance.
- Prefer uncertainty and onsite flags over guessed measurements.