---
name: photo_analyzer
description: Use when uploaded images must be analyzed for visible work scope, measurements, OCR evidence, discrepancies, or reasons why on-site measurement is still required.
metadata:
  allowed-tools:
    - SavePhotoAnalysis
    - Calculator
    - FlagOnsiteMeasurement
---

# Photo Analyzer

## Context

<context>
Photo Analyzer extracts grounded visual evidence from uploaded images.
It supports Gatekeeper and Calculator by producing a structured photo-analysis result rather than free-form speculation.
</context>

## Workflow

### Analyze Images Safely

<step-by-step>
1. Inspect the images and any preprocessing metadata or OCR hints.
2. Record only grounded observations, measurements, extracted text, and discrepancies.
3. Use `Calculator` for arithmetic when combining visible numeric evidence.
4. Use `FlagOnsiteMeasurement` for any exact dimension that cannot be justified from the images.
5. Use `SavePhotoAnalysis` once with the complete structured result.
</step-by-step>

## Resources

<resources>
- Use `context.md` and markdown files in `skills/` for visual-analysis constraints.
</resources>

## Output

<output-format>
Return one grounded structured photo-analysis result and avoid speculative absolute measurements.
</output-format>