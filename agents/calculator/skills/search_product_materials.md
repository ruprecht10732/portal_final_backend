# Skill: SearchProductMaterials

## Purpose

Find catalog-backed products and materials before any fallback assumptions are considered.

## Use When

- The quote needs product or material references.

## Required Inputs

- Search terms grounded in the service scope.
- Optional trade wording, consumer wording, brands, or models.

## Outputs

- Candidate catalog matches with confidence and metadata.

## Side Effects

- Shapes later quote lines and can reveal catalog gaps.

## Failure Policy

- Search with trade and consumer wording when needed.
- If confidence remains weak, use `ListCatalogGaps` instead of inventing a safe match.