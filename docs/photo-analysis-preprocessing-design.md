# Photo Analysis Preprocessing Design

## Status

Partially implemented.

Currently live in the backend runtime path:

- a fail-open `ImagePreprocessor` integration before `PhotoAnalyzer`
- file-level image metadata extraction and best-effort EXIF capture
- a generated grayscale structural variant with mild contrast enhancement for decodable formats
- optional OCR assist candidates via `tesseract` when enabled and available
- service-type-gated best-effort lens correction behind a rollout flag
- service-type-gated best-effort perspective normalization behind a rollout flag
- preprocessing context injected into the analyzer prompt
- preprocessing provenance written into photo-analysis timeline metadata

Still pending for later phases:

- confidence-scored OCR tokens with bounding boxes
- camera-model-aware lens correction profiles beyond the current best-effort pass
- stronger rectangular corner detection for perspective normalization
- dedicated persistence model for preprocessing provenance beyond timeline metadata
- frontend surfaces for preprocessing provenance beyond existing analysis badges

## Problem

Raw smartphone images are a poor source for absolute measurement inference. Wide-angle distortion, perspective skew, motion blur, low contrast, and inconsistent framing reduce reliability before the vision model starts reasoning. Prompt hardening reduces bad inferences, but it does not improve the underlying image signal.

## Goals

- Improve the reliability of visual evidence before images reach the PhotoAnalyzer.
- Preserve the original upload so preprocessing is reversible and auditable.
- Keep preprocessing optional and non-blocking during rollout.
- Produce metadata that downstream prompts and UI can use to explain why a finding is OCR-backed or still requires on-site verification.

## Non-goals

- Do not turn 2D photos into trusted survey-grade measurements.
- Do not replace on-site verification for pricing-critical dimensions.
- Do not block lead intake when preprocessing fails.

## Proposed Pipeline

1. Attachment download.
2. Image metadata extraction.
3. Deterministic preprocessing transforms.
4. Store original image, processed variants, and preprocessing metadata.
5. Send the processed package plus metadata to PhotoAnalyzer.
6. Persist both photo-analysis result and preprocessing provenance.

## Preprocessing Stages

### 1. Metadata extraction

Capture EXIF and file-level metadata when available:

- camera make and model
- focal length
- orientation
- resolution
- timestamp

If EXIF is missing, continue with best-effort defaults and mark provenance accordingly.

### 2. Lens correction

Apply camera-model-aware lens correction when focal data is available. If the device profile is unknown, skip the correction and record that the transform was unavailable.

Expected outcome:

- straighter architectural lines
- less barrel distortion at the frame edges
- better downstream OCR placement on labels and plates

### 3. Perspective normalization

Attempt perspective rectification when the image contains strong rectangular references such as:

- doors
- windows
- wall openings
- floor tile grids
- equipment front faces

If corner detection confidence is low, do not warp the image. Record the reason in preprocessing metadata.

### 4. Contrast and edge enhancement

Generate at most one additional derived image tuned for structural visibility:

- local contrast enhancement for dim spaces
- mild sharpening for edges
- optional edge map for countable fixtures or trim lines

The analyzer should receive both the original color image and a derived structural variant when available.

### 5. OCR assist layer

Run a lightweight OCR pass before full analysis and store:

- extracted text candidates
- bounding boxes when available
- confidence per token or block

The PhotoAnalyzer can use this as supporting evidence instead of rediscovering all text from scratch.

## Service Shape

Introduce a new internal preprocessing service with this responsibility split:

- PhotoAnalysisHandler remains the orchestrator entry point.
- ImagePreprocessor prepares variants and metadata.
- PhotoAnalyzer consumes original plus processed assets.
- Repository persists preprocessing provenance separately from analysis conclusions.

Suggested interface:

```go
type ImagePreprocessor interface {
    Prepare(ctx context.Context, images []ImageData) ([]PreparedImage, error)
}

type PreparedImage struct {
    Original      ImageData
    Variants      []ImageVariant
    Metadata      PreprocessingMetadata
    OCRCandidates []OCRCandidate
}
```

## Storage and Provenance

Store preprocessing provenance so the system can explain what evidence was machine-enhanced:

- original attachment id
- generated variant ids
- transforms applied
- skipped transforms with reasons
- OCR confidence summary
- preprocessing timestamp

This should support later UI cues such as:

- OCR-backed
- perspective-normalized
- on-site verification required

## Prompt Integration

Once preprocessing exists, extend the PhotoAnalyzer prompt to consume provenance explicitly:

- prefer OCR-backed text over freeform reading
- treat normalized variants as supporting evidence, not ground truth
- keep FlagOnsiteMeasurement mandatory for pricing-critical dimensions

## Rollout Plan

### Phase 1

Implemented.

- `ImagePreprocessor` interface added
- fail-open basic preprocessor wired into `PhotoAnalysisHandler`
- enabled by default through tenant-scoped organization AI settings
- metadata extraction and structural variant generation included in analyzer context
- failures fall back to original images without blocking analysis

### Phase 2

Implemented in best-effort form.

- OCR assist can run through local `tesseract` when enabled in organization AI settings
- OCR assist can be narrowed by tenant-scoped service-type allowlists
- extracted OCR candidates are injected into analyzer context before full vision reasoning

### Phase 3

Implemented behind tenant-scoped rollout settings.

- lens correction enablement and service-type allowlists live in organization AI settings
- perspective normalization enablement and service-type allowlists live in organization AI settings

Current transforms are best-effort and fail open:

- lens correction uses focal-length-based radial remapping when EXIF focal length exists
- perspective normalization uses row-bound keystone detection and linear normalization when a strong top/bottom width delta is detected

### Phase 4

Expose preprocessing provenance in internal debugging UI and evaluate its impact on quote-quality metrics.

## Operational Control

Implemented.

- preprocessing, OCR assist, lens correction, and perspective normalization now live in `RAC_organization_settings`
- settings are exposed through the tenant-scoped identity settings API and mapped into leads via `OrganizationAISettingsReader`
- the frontend organization AI settings page can edit each toggle and service-type allowlist

## Success Metrics

- lower rate of estimator follow-up caused by unreadable labels
- higher share of OCR-backed product identification
- fewer unsafe dimension guesses in photo-analysis output
- no increase in failed photo-analysis jobs during rollout

## Open Questions

- Which library stack is acceptable in production for image transforms and OCR?
- Should preprocessing run inline or on the existing scheduler queue?
- How long should generated variants be retained?
- Which service types benefit enough to justify perspective normalization?