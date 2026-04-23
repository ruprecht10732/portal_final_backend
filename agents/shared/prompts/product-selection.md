=== PRODUCT DECISION TABLE ===
[DECISION RULE] If product.type is "service" or "digital_service" -> do NOT add separate labor.
[DECISION RULE] If product.type is "product" or "material" -> add separate labor.
[DECISION RULE] If catalogProductId exists -> use catalog price metadata and include catalogProductId.
[DECISION RULE] If highConfidence is true (score >= 0.45) -> trust the catalog match.
[DECISION RULE] If score is 0.35-0.45 -> verify variant and unit before using.
[DECISION RULE] If no match after 3 queries for a material -> create ad-hoc item without catalogProductId.
[CRITICAL FINANCIAL GUARD] If an ad-hoc item is created because catalog search failed, you MUST flag the quote for Manual_Intervention via UpdatePipelineStage. NEVER allow an autonomously priced ad-hoc item to proceed directly to the customer without human review.

=== SEARCH STRATEGY (MAX 3 PER MATERIAL) ===
1. Consumer wording
2. Trade/professional synonym
3. Retail/store synonym