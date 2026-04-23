package agent

import (
	"context"
	"fmt"
	"log"
	"math"
	"portal_final_backend/internal/leads/ports"
	"portal_final_backend/internal/leads/repository"
	"portal_final_backend/platform/qdrant"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/adk/tool"
	apptools "portal_final_backend/internal/tools"
)

func noMatchMessage(query string) string {
	return fmt.Sprintf("No relevant products found for query '%s'. Try different search terms (synonyms, broader/narrower terms, Dutch and English). If no match exists, you may add an ad-hoc item.", query)
}

func recordCatalogSearch(ctx context.Context, deps *ToolDependencies, query string, collection string, resultCount int, topScore *float64) {
	tenantID, ok := deps.GetTenantID()
	if !ok || tenantID == nil {
		return
	}

	_, serviceID, hasLeadCtx := deps.GetLeadContext()
	var servicePtr *uuid.UUID
	if hasLeadCtx {
		sid := serviceID
		servicePtr = &sid
	}

	if deps.Repo == nil {
		return
	}
	runID := strings.TrimSpace(deps.GetRunID())
	var runIDPtr *string
	if runID != "" {
		runIDPtr = &runID
	}
	_, actorName := deps.GetActor()
	actorName = strings.TrimSpace(actorName)
	var agentNamePtr *string
	if actorName != "" {
		agentNamePtr = &actorName
	}
	toolName := "SearchProductMaterials"
	if err := deps.Repo.CreateCatalogSearchLog(ctx, repository.CreateCatalogSearchLogParams{
		OrganizationID: *tenantID,
		LeadServiceID:  servicePtr,
		RunID:          runIDPtr,
		ToolName:       &toolName,
		AgentName:      agentNamePtr,
		Query:          query,
		Collection:     collection,
		ResultCount:    resultCount,
		TopScore:       topScore,
	}); err != nil {
		log.Printf("SearchProductMaterials: failed to write catalog search log: %v", err)
	}
}

type ListCatalogGapsInput struct {
	// LookbackDays defaults to organization setting catalogGapLookbackDays.
	LookbackDays *int `json:"lookbackDays,omitempty"`
	// MinCount defaults to organization setting catalogGapThreshold.
	MinCount *int `json:"minCount,omitempty"`
	// Limit defaults to 25.
	Limit *int `json:"limit,omitempty"`
}

type CatalogSearchMissSummaryDTO struct {
	Query       string    `json:"query"`
	SearchCount int       `json:"searchCount"`
	LastSeenAt  time.Time `json:"lastSeenAt"`
	Collections []string  `json:"collections"`
}

type AdHocQuoteItemSummaryDTO struct {
	Description string    `json:"description"`
	UseCount    int       `json:"useCount"`
	LastSeenAt  time.Time `json:"lastSeenAt"`
}

type ListCatalogGapsOutput struct {
	LookbackDays    int                           `json:"lookbackDays"`
	MinCount        int                           `json:"minCount"`
	SearchMisses    []CatalogSearchMissSummaryDTO `json:"searchMisses"`
	AdHocQuoteItems []AdHocQuoteItemSummaryDTO    `json:"adHocQuoteItems"`
	Message         string                        `json:"message,omitempty"`
}

type listCatalogGapsParams struct {
	lookbackDays int
	minCount     int
	limit        int
}

func resolveOptionalIntWithin(defaultVal int, override *int, minVal int, maxVal int) int {
	val := defaultVal
	if override != nil {
		val = *override
	}
	if val < minVal {
		return minVal
	}
	if val > maxVal {
		return maxVal
	}
	return val
}

func resolveListCatalogGapsParams(settings ports.OrganizationAISettings, input ListCatalogGapsInput) listCatalogGapsParams {
	lookbackDays := resolveOptionalIntWithin(settings.CatalogGapLookbackDays, input.LookbackDays, 1, 365)
	minCount := resolveOptionalIntWithin(settings.CatalogGapThreshold, input.MinCount, 1, 1000)

	limit := 25
	if input.Limit != nil {
		limit = normalizeLimit(*input.Limit, 25, 100)
	}

	return listCatalogGapsParams{lookbackDays: lookbackDays, minCount: minCount, limit: limit}
}

func mapCatalogSearchMissSummaries(misses []repository.CatalogSearchMissSummary) []CatalogSearchMissSummaryDTO {
	out := make([]CatalogSearchMissSummaryDTO, 0, len(misses))
	for _, m := range misses {
		out = append(out, CatalogSearchMissSummaryDTO{
			Query:       m.Query,
			SearchCount: m.SearchCount,
			LastSeenAt:  m.LastSeenAt,
			Collections: m.Collections,
		})
	}
	return out
}

func mapAdHocQuoteItemSummaries(items []repository.AdHocQuoteItemSummary) []AdHocQuoteItemSummaryDTO {
	out := make([]AdHocQuoteItemSummaryDTO, 0, len(items))
	for _, it := range items {
		out = append(out, AdHocQuoteItemSummaryDTO{
			Description: it.Description,
			UseCount:    it.UseCount,
			LastSeenAt:  it.LastSeenAt,
		})
	}
	return out
}

func buildListCatalogGapsOutput(params listCatalogGapsParams, misses []repository.CatalogSearchMissSummary, adHoc []repository.AdHocQuoteItemSummary) ListCatalogGapsOutput {
	out := ListCatalogGapsOutput{
		LookbackDays:    params.lookbackDays,
		MinCount:        params.minCount,
		SearchMisses:    mapCatalogSearchMissSummaries(misses),
		AdHocQuoteItems: mapAdHocQuoteItemSummaries(adHoc),
	}

	if len(out.SearchMisses) == 0 && len(out.AdHocQuoteItems) == 0 {
		out.Message = "No frequent catalog gaps detected in the selected lookback window."
	}

	return out
}

func listCatalogGapsErrorOutput(params listCatalogGapsParams, message string) ListCatalogGapsOutput {
	return ListCatalogGapsOutput{LookbackDays: params.lookbackDays, MinCount: params.minCount, Message: message}
}

func handleListCatalogGaps(ctx tool.Context, deps *ToolDependencies, input ListCatalogGapsInput) (ListCatalogGapsOutput, error) {
	tenantID, ok := deps.GetTenantID()
	if !ok || tenantID == nil {
		return ListCatalogGapsOutput{Message: missingTenantContextMessage}, nil
	}
	if deps.Repo == nil {
		return ListCatalogGapsOutput{Message: "Repository not configured"}, nil
	}

	params := resolveListCatalogGapsParams(deps.GetOrganizationAISettingsOrDefault(), input)

	misses, err := deps.Repo.ListFrequentCatalogSearchMisses(ctx, *tenantID, params.lookbackDays, params.minCount, params.limit)
	if err != nil {
		log.Printf("ListCatalogGaps: failed to list catalog search misses: %v", err)
		return listCatalogGapsErrorOutput(params, "Failed to load catalog search misses"), nil
	}

	adHoc, err := deps.Repo.ListFrequentAdHocQuoteItems(ctx, *tenantID, params.lookbackDays, params.minCount, params.limit)
	if err != nil {
		log.Printf("ListCatalogGaps: failed to list ad-hoc quote items: %v", err)
		return listCatalogGapsErrorOutput(params, "Failed to load ad-hoc quote items"), nil
	}

	return buildListCatalogGapsOutput(params, misses, adHoc), nil
}

func createListCatalogGapsTool(_ *ToolDependencies) (tool.Tool, error) {
	return apptools.NewListCatalogGapsTool(func(ctx tool.Context, input ListCatalogGapsInput) (ListCatalogGapsOutput, error) {
		deps, err := GetDependencies(ctx)
		if err != nil {
			return ListCatalogGapsOutput{}, err
		}
		return handleListCatalogGaps(ctx, deps, input)
	})
}

// resolveSearchParams extracts and normalises the search parameters from input.
func resolveSearchParams(input SearchProductMaterialsInput) (query string, limit int, useCatalog bool, scoreThreshold float64, err error) {
	query = strings.TrimSpace(input.Query)
	if query == "" {
		return "", 0, false, 0, fmt.Errorf("empty query")
	}
	limit = normalizeLimit(input.Limit, 5, 20)
	useCatalog = true
	if input.UseCatalog != nil {
		useCatalog = *input.UseCatalog
	}
	scoreThreshold = defaultSearchScoreThreshold
	if input.MinScore != nil && *input.MinScore > 0 && *input.MinScore < 1 {
		scoreThreshold = *input.MinScore
	}
	return query, limit, useCatalog, scoreThreshold, nil
}

// searchCatalogCollection searches the catalog Qdrant collection and hydrates results.
// Returns an error when the underlying vector search fails so callers can abort rather
// than hallucinate ad-hoc products.
func searchCatalogCollection(ctx tool.Context, deps *ToolDependencies, vector []float32, limit int, scoreThreshold float64, query string) ([]ProductResult, error) {
	tenantID, tenantOk := deps.GetTenantID()
	var filter *qdrant.Filter
	if tenantOk && tenantID != nil {
		filter = qdrant.NewOrganizationFilter(tenantID.String())
		log.Printf("SearchProductMaterials: catalog search with tenant filter organization_id=%s", tenantID.String())
	} else {
		log.Printf("SearchProductMaterials: catalog search without tenant filter (missing tenant context)")
	}

	searchCtx, searchCancel := detachedTimeout(ctx, toolIOTimeout)
	defer searchCancel()
	results, err := deps.CatalogQdrantClient.SearchWithFilter(searchCtx, vector, limit, scoreThreshold, filter)
	if err != nil {
		log.Printf("SearchProductMaterials: catalog search failed: %v", err)
		recordCatalogSearch(ctx, deps, query, "catalog", 0, nil)
		return nil, err
	}
	var topScore *float64
	if len(results) > 0 {
		s := results[0].Score
		topScore = &s
	}
	products := convertSearchResults(results)
	recordCatalogSearch(ctx, deps, query, "catalog", len(products), topScore)
	if len(products) == 0 {
		log.Printf("SearchProductMaterials: catalog query=%q found 0 products above threshold %.2f, falling back", query, scoreThreshold)
		return nil, nil
	}
	products = hydrateProductResults(ctx, deps, products)
	products = rerankCatalogProducts(query, products)
	markHighConfidence(products)
	logCatalogSelectionAudit(query, products)
	log.Printf("SearchProductMaterials: catalog query=%q found %d products (threshold=%.2f, scores: %s)",
		query, len(products), scoreThreshold, formatScores(products))
	return products, nil
}

func handleSearchProductMaterials(ctx tool.Context, deps *ToolDependencies, input SearchProductMaterialsInput) (SearchProductMaterialsOutput, error) {
	if !deps.IsProductSearchEnabled() {
		return SearchProductMaterialsOutput{Products: nil, Message: "Product search is not configured"}, nil
	}

	query, limit, useCatalog, scoreThreshold, err := resolveSearchParams(input)
	if err != nil {
		return SearchProductMaterialsOutput{Products: nil, Message: "Query cannot be empty"}, err
	}

	cacheKey := fmt.Sprintf("%s|%d|%t|%.4f", strings.ToLower(strings.TrimSpace(query)), limit, useCatalog, scoreThreshold)
	if cached, ok := deps.getSearchCache(cacheKey); ok {
		log.Printf("SearchProductMaterials: cache hit query=%q limit=%d useCatalog=%t minScore=%.2f", query, limit, useCatalog, scoreThreshold)
		return cached, nil
	}

	normKey := normalizedSearchCacheKey(query, limit, useCatalog, scoreThreshold)
	if cached, ok := deps.getSearchCacheNormalized(normKey); ok {
		log.Printf("SearchProductMaterials: normalized cache hit query=%q limit=%d useCatalog=%t minScore=%.2f", query, limit, useCatalog, scoreThreshold)
		return cached, nil
	}

	embedCtx, embedCancel := detachedTimeout(ctx, toolIOTimeout)
	defer embedCancel()
	vector, err := deps.EmbeddingClient.Embed(embedCtx, query)
	if err != nil {
		log.Printf("SearchProductMaterials: embedding failed: %v", err)
		return SearchProductMaterialsOutput{Products: nil, Message: "Failed to generate embedding for query"}, err
	}

	catalogOutput, foundInCatalog, catalogErr := tryCatalogSearchFlow(ctx, deps, query, limit, scoreThreshold, useCatalog, vector)
	if catalogErr != nil {
		return SearchProductMaterialsOutput{Products: nil, Message: "Product catalog search failed"}, catalogErr
	}
	setBothCaches := func(output SearchProductMaterialsOutput) {
		deps.setSearchCache(cacheKey, output)
		deps.setSearchCache(normKey, output)
	}

	if foundInCatalog && hasStrongCatalogMatch(catalogOutput.Products) {
		setBothCaches(catalogOutput)
		return catalogOutput, nil
	}

	fallbackOutput, fallbackErr := searchFallbackReferenceCollections(ctx, deps, query, vector, limit, scoreThreshold)
	if fallbackErr != nil {
		if foundInCatalog && len(catalogOutput.Products) > 0 {
			log.Printf("SearchProductMaterials: fallback search failed, returning catalog-only low-confidence results: %v", fallbackErr)
			setBothCaches(catalogOutput)
			return catalogOutput, nil
		}
		return fallbackOutput, fallbackErr
	}

	if foundInCatalog && len(catalogOutput.Products) > 0 {
		if len(fallbackOutput.Products) == 0 {
			setBothCaches(catalogOutput)
			return catalogOutput, nil
		}

		log.Printf("SearchProductMaterials: catalog had no high-confidence matches, adding fallback collections")
		combinedOutput := combineCatalogAndFallbackResults(catalogOutput, fallbackOutput, query, scoreThreshold, limit)
		setBothCaches(combinedOutput)
		return combinedOutput, nil
	}

	setBothCaches(fallbackOutput)
	return fallbackOutput, nil
}

func searchFallbackReferenceCollections(ctx tool.Context, deps *ToolDependencies, query string, vector []float32, limit int, scoreThreshold float64) (SearchProductMaterialsOutput, error) {

	// Fallback to reference collections.
	if deps.QdrantClient == nil && deps.BouwmaatQdrantClient == nil {
		return SearchProductMaterialsOutput{Products: nil, Message: noMatchMessage(query)}, nil
	}

	batchClient := resolveFallbackBatchClient(deps)
	batchRequests, requestCollections := buildFallbackBatchRequests(deps, vector, limit, scoreThreshold)

	batchCtx, batchCancel := detachedTimeout(ctx, toolIOTimeout)
	defer batchCancel()
	batchResults, err := batchClient.BatchSearch(batchCtx, batchRequests)
	if err != nil {
		log.Printf("SearchProductMaterials: fallback batch search failed: %v", err)
		return SearchProductMaterialsOutput{Products: nil, Message: "Failed to search product catalog"}, err
	}

	products := flattenFallbackBatchResults(ctx, deps, query, batchResults, requestCollections, limit)
	return buildFallbackSearchOutput(query, products, requestCollections, scoreThreshold), nil
}

func resolveFallbackBatchClient(deps *ToolDependencies) *qdrant.Client {
	if deps.QdrantClient != nil {
		return deps.QdrantClient
	}
	return deps.BouwmaatQdrantClient
}

func buildFallbackBatchRequests(deps *ToolDependencies, vector []float32, limit int, scoreThreshold float64) ([]qdrant.SearchRequest, []string) {
	batchRequests := make([]qdrant.SearchRequest, 0, 2)
	requestCollections := make([]string, 0, 2)

	if deps.QdrantClient != nil {
		houthandelCollection := deps.QdrantClient.CollectionName()
		if houthandelCollection == "" {
			houthandelCollection = defaultHouthandelCollection
		}
		batchRequests = append(batchRequests, newFallbackBatchRequest(houthandelCollection, vector, limit, scoreThreshold))
		requestCollections = append(requestCollections, houthandelCollection)
	}

	if deps.BouwmaatQdrantClient != nil {
		bouwmaatCollection := deps.BouwmaatQdrantClient.CollectionName()
		if bouwmaatCollection == "" {
			bouwmaatCollection = defaultBouwmaatCollection
		}
		batchRequests = append(batchRequests, newFallbackBatchRequest(bouwmaatCollection, vector, limit, scoreThreshold))
		requestCollections = append(requestCollections, bouwmaatCollection)
	}

	return batchRequests, requestCollections
}

func newFallbackBatchRequest(collectionName string, vector []float32, limit int, scoreThreshold float64) qdrant.SearchRequest {
	return qdrant.SearchRequest{
		CollectionName: collectionName,
		Vector:         vector,
		Limit:          limit,
		WithPayload:    true,
		ScoreThreshold: &scoreThreshold,
	}
}

func flattenFallbackBatchResults(ctx tool.Context, deps *ToolDependencies, query string, batchResults [][]qdrant.SearchResult, requestCollections []string, limit int) []ProductResult {
	products := make([]ProductResult, 0, limit*len(batchResults))
	for idx, results := range batchResults {
		collectionName := "unknown"
		if idx < len(requestCollections) {
			collectionName = requestCollections[idx]
		}
		var topScore *float64
		if len(results) > 0 {
			s := results[0].Score
			topScore = &s
		}
		collectionProducts := convertSearchResults(results)
		recordCatalogSearch(ctx, deps, query, collectionName, len(collectionProducts), topScore)
		for i := range collectionProducts {
			collectionProducts[i].SourceCollection = collectionName
		}
		products = append(products, collectionProducts...)
		log.Printf("SearchProductMaterials: fallback batch query=%q collection=%s results=%d", query, collectionName, len(collectionProducts))
	}

	sort.SliceStable(products, func(i, j int) bool {
		if products[i].Score == products[j].Score {
			return products[i].PriceEuros < products[j].PriceEuros
		}
		return products[i].Score > products[j].Score
	})

	return products
}

func buildFallbackSearchOutput(query string, products []ProductResult, requestCollections []string, scoreThreshold float64) SearchProductMaterialsOutput {
	markHighConfidence(products)
	if len(products) == 0 {
		log.Printf("SearchProductMaterials: fallback batch query=%q found 0 products above threshold %.2f", query, scoreThreshold)
		return SearchProductMaterialsOutput{Products: nil, Message: noMatchMessage(query)}
	}

	// Fallback results are scraped reference data — strip IDs so the AI
	// treats them as ad-hoc line items (no catalogProductId, no auto-attachments).
	stripProductIDs(products)

	log.Printf("SearchProductMaterials: fallback batch query=%q found %d reference products across %d collections (threshold=%.2f, scores: %s)",
		query, len(products), len(requestCollections), scoreThreshold, formatScores(products))

	log.Printf("SearchProductMaterials: fallback collections=%s", strings.Join(requestCollections, ","))

	return SearchProductMaterialsOutput{
		Products: products,
		Message:  fmt.Sprintf("Found %d reference products (not from your catalog — use as ad-hoc line items without catalogProductId, min relevance %.0f%%)", len(products), scoreThreshold*100),
	}
}

func combineCatalogAndFallbackResults(catalogOutput SearchProductMaterialsOutput, fallbackOutput SearchProductMaterialsOutput, query string, scoreThreshold float64, limit int) SearchProductMaterialsOutput {
	products := make([]ProductResult, 0, len(catalogOutput.Products)+len(fallbackOutput.Products))
	products = append(products, catalogOutput.Products...)
	products = append(products, fallbackOutput.Products...)

	sort.SliceStable(products, func(i, j int) bool {
		if products[i].Score == products[j].Score {
			return products[i].PriceEuros < products[j].PriceEuros
		}
		return products[i].Score > products[j].Score
	})

	if len(products) > limit {
		products = products[:limit]
	}

	catalogCount := len(catalogOutput.Products)
	fallbackCount := len(fallbackOutput.Products)

	log.Printf("SearchProductMaterials: combined query=%q catalog=%d fallback=%d total=%d (threshold=%.2f)",
		query, catalogCount, fallbackCount, len(products), scoreThreshold)

	return SearchProductMaterialsOutput{
		Products: products,
		Message: fmt.Sprintf(
			"Found %d products: %d catalog + %d fallback references (catalog is lower confidence; verify variant/unit before drafting, min relevance %.0f%%)",
			len(products),
			catalogCount,
			fallbackCount,
			scoreThreshold*100,
		),
	}
}

func tryCatalogSearchFlow(ctx tool.Context, deps *ToolDependencies, query string, limit int, scoreThreshold float64, useCatalog bool, initialVector []float32) (SearchProductMaterialsOutput, bool, error) {
	if !useCatalog || deps.CatalogQdrantClient == nil {
		return SearchProductMaterialsOutput{}, false, nil
	}

	initialProducts, err := searchCatalogCollection(ctx, deps, initialVector, limit, scoreThreshold, query)
	if err != nil {
		return SearchProductMaterialsOutput{}, false, err
	}
	if len(initialProducts) > 0 {
		if hasHighConfidenceMatch(initialProducts) {
			// Original query produced a genuine high-confidence match — authoritative.
			return catalogSearchOutput(initialProducts, scoreThreshold, false, true), true, nil
		}

		bestProducts, highConfidenceProducts, _ := runCatalogRetries(ctx, deps, query, limit, scoreThreshold, initialProducts)
		if len(highConfidenceProducts) > 0 {
			// Retry improved confidence but the original query did NOT have high
			// confidence. Return products but mark highConfidence=false so the
			// caller still tries fallback reference collections.
			return catalogSearchOutput(highConfidenceProducts, scoreThreshold, true, false), true, nil
		}
		return catalogSearchOutput(bestProducts, scoreThreshold, false, false), true, nil
	}

	bestRetryProducts, highConfidenceProducts, _ := runCatalogRetries(ctx, deps, query, limit, scoreThreshold, nil)
	if len(highConfidenceProducts) > 0 {
		// Only retries found something — not authoritative enough to skip fallback.
		return catalogSearchOutput(highConfidenceProducts, scoreThreshold, true, false), true, nil
	}
	if len(bestRetryProducts) > 0 {
		return catalogSearchOutput(bestRetryProducts, scoreThreshold, true, false), true, nil
	}

	return SearchProductMaterialsOutput{}, false, nil
}

func catalogSearchOutput(products []ProductResult, scoreThreshold float64, reworded bool, highConfidence bool) SearchProductMaterialsOutput {
	if highConfidence {
		if reworded {
			return SearchProductMaterialsOutput{
				Products: products,
				Message:  fmt.Sprintf("Found %d high-confidence matching products from catalog after query rewording (min relevance %.0f%%)", len(products), scoreThreshold*100),
			}
		}
		return SearchProductMaterialsOutput{
			Products: products,
			Message:  fmt.Sprintf("Found %d high-confidence matching products from catalog (min relevance %.0f%%)", len(products), scoreThreshold*100),
		}
	}

	if reworded {
		return SearchProductMaterialsOutput{
			Products: products,
			Message:  fmt.Sprintf("Found %d matching products from catalog after query rewording (lower confidence; verify variant/unit, min relevance %.0f%%)", len(products), scoreThreshold*100),
		}
	}

	return SearchProductMaterialsOutput{
		Products: products,
		Message:  fmt.Sprintf("Found %d matching products from catalog (lower confidence; verify variant/unit, min relevance %.0f%%)", len(products), scoreThreshold*100),
	}
}

func limitedCatalogRewordedQueries(query string) []string {
	rewordedQueries := buildCatalogRewordedQueries(query)
	if len(rewordedQueries) > maxCatalogRewordRetries {
		return rewordedQueries[:maxCatalogRewordRetries]
	}
	return rewordedQueries
}

func runCatalogRetries(ctx tool.Context, deps *ToolDependencies, query string, limit int, scoreThreshold float64, currentBest []ProductResult) (bestProducts []ProductResult, highConfidenceProducts []ProductResult, usedRewording bool) {
	bestProducts = currentBest
	for _, retryQuery := range limitedCatalogRewordedQueries(query) {
		retryProducts, retryErr := searchCatalogRetryQuery(ctx, deps, retryQuery, limit, scoreThreshold)
		if retryErr != nil {
			log.Printf("SearchProductMaterials: catalog retry query=%q search failed: %v", retryQuery, retryErr)
			continue
		}
		if len(retryProducts) == 0 {
			continue
		}

		usedRewording = true
		if shouldPreferCandidateSet(retryProducts, bestProducts) {
			bestProducts = retryProducts
		}

		if hasHighConfidenceMatch(retryProducts) {
			log.Printf("SearchProductMaterials: catalog retry improved confidence query=%q -> retry_query=%q", query, retryQuery)
			return bestProducts, retryProducts, true
		}
	}

	return bestProducts, nil, usedRewording
}

func searchCatalogRetryQuery(ctx tool.Context, deps *ToolDependencies, retryQuery string, limit int, scoreThreshold float64) ([]ProductResult, error) {
	retryEmbedCtx, retryEmbedCancel := detachedTimeout(ctx, toolIOTimeout)
	defer retryEmbedCancel()
	retryVector, retryErr := deps.EmbeddingClient.Embed(retryEmbedCtx, retryQuery)
	if retryErr != nil {
		log.Printf("SearchProductMaterials: catalog retry embedding failed query=%q: %v", retryQuery, retryErr)
		return nil, retryErr
	}
	return searchCatalogCollection(ctx, deps, retryVector, limit, scoreThreshold, retryQuery)
}

func hasHighConfidenceMatch(products []ProductResult) bool {
	for _, product := range products {
		if product.HighConfidence {
			return true
		}
	}
	return false
}

func hasStrongCatalogMatch(products []ProductResult) bool {
	for _, product := range products {
		if product.Score >= catalogEarlyReturnScoreThreshold {
			return true
		}
	}
	return false
}

func shouldPreferCandidateSet(candidate []ProductResult, current []ProductResult) bool {
	if len(candidate) == 0 {
		return false
	}
	if len(current) == 0 {
		return true
	}
	candidateHigh := hasHighConfidenceMatch(candidate)
	currentHigh := hasHighConfidenceMatch(current)
	if candidateHigh != currentHigh {
		return candidateHigh
	}
	return candidate[0].Score > current[0].Score
}

func buildCatalogRewordedQueries(query string) []string {
	base := strings.TrimSpace(strings.ToLower(query))
	if base == "" {
		return nil
	}

	queries := make([]string, 0, 4)
	appendUniqueQuery := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		for _, existing := range queries {
			if existing == value {
				return
			}
		}
		queries = append(queries, value)
	}

	synonymExpansions := map[string]string{
		"kantstuk":      "dagkantafwerking deurlijst chambranle aftimmerlat afdeklat kozijnplint sponninglat",
		"kantstukken":   "dagkantafwerking deurlijst chambranle aftimmerlat afdeklat kozijnplint sponninglat",
		"zweeds rabat":  "potdekselplank gevelbekleding rabatdeel",
		"grondverf":     "primer hout grondlaag",
		"randsealer":    "kanten sealer randafdichting",
		"paal":          "staander tuinpaal",
		"angelim":       "hardhout paal tropisch",
		"geimpregneerd": "druk geimpregneerd buitenhout",
	}

	for key, expansion := range synonymExpansions {
		if strings.Contains(base, key) {
			appendUniqueQuery(base + " " + expansion)
		}
	}

	without := strings.ReplaceAll(base, " inclusief ", " ")
	if without != base {
		appendUniqueQuery(without)
	}

	return queries
}

// stripProductIDs clears the ID field on all products so the AI treats
// them as ad-hoc items (no catalogProductId on the draft quote).
// Also sets a default VAT rate of 21% for fallback products that lack one.
func stripProductIDs(products []ProductResult) {
	for i := range products {
		products[i].ID = ""
		if products[i].VatRateBps == 0 {
			products[i].VatRateBps = 2100 // 21% BTW default
		}
	}
}

// formatScores returns a compact summary of product scores for logging.
func formatScores(products []ProductResult) string {
	if len(products) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(products))
	for _, p := range products {
		parts = append(parts, fmt.Sprintf("%.3f", p.Score))
	}
	return strings.Join(parts, ", ")
}

func normalizeLimit(limit, defaultVal, maxVal int) int {
	if limit <= 0 {
		return defaultVal
	}
	if limit > maxVal {
		return maxVal
	}
	return limit
}

func markHighConfidence(products []ProductResult) {
	for i := range products {
		products[i].HighConfidence = products[i].Score >= highConfidenceScoreThreshold
	}
}

func truncateRunes(value string, max int) string {
	if max <= 0 || value == "" {
		return ""
	}
	if len(value) <= max {
		return value
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}

func convertSearchResults(results []qdrant.SearchResult) []ProductResult {
	products := make([]ProductResult, 0, len(results))
	for _, r := range results {
		product := extractProductFromPayload(r.Payload, r.Score)
		if product.Name != "" {
			products = append(products, product)
		}
	}

	// Default ordering: strongest semantic matches first.
	sort.SliceStable(products, func(i, j int) bool {
		if products[i].Score == products[j].Score {
			return products[i].PriceEuros < products[j].PriceEuros
		}
		return products[i].Score > products[j].Score
	})
	return products
}

func rerankCatalogProducts(query string, products []ProductResult) []ProductResult {
	if len(products) <= 1 {
		return products
	}

	queryTokens := tokenizeForMatch(query)
	queryDims := extractDimensionTokens(query)
	queryUnits := extractUnitTokens(query)

	type rankedProduct struct {
		product    ProductResult
		rankScore  float64
		overlap    float64
		dimMatches int
		unitMatch  bool
	}

	ranked := make([]rankedProduct, 0, len(products))
	for _, product := range products {
		text := strings.ToLower(strings.Join([]string{product.Name, product.Description, product.Unit, product.Category}, " "))
		textTokens := tokenizeForMatch(text)
		overlap := tokenOverlapRatio(queryTokens, textTokens)
		dimMatches := countSetIntersection(queryDims, extractDimensionTokens(text))
		unitMatch := hasAnyUnitToken(text, queryUnits)

		rank := product.Score*1000 + overlap*120 + float64(dimMatches)*30
		if unitMatch {
			rank += 20
		}

		ranked = append(ranked, rankedProduct{
			product:    product,
			rankScore:  rank,
			overlap:    overlap,
			dimMatches: dimMatches,
			unitMatch:  unitMatch,
		})
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].rankScore == ranked[j].rankScore {
			return ranked[i].product.Score > ranked[j].product.Score
		}
		return ranked[i].rankScore > ranked[j].rankScore
	})

	for i := range products {
		products[i] = ranked[i].product
	}

	return products
}

func logCatalogSelectionAudit(query string, products []ProductResult) {
	if len(products) == 0 {
		return
	}

	highConfidenceCount := 0
	for _, product := range products {
		if product.HighConfidence {
			highConfidenceCount++
		}
	}

	top := products[0]
	log.Printf(
		"SearchProductMaterials: catalog selection audit query=%q top_id=%q top_name=%q top_score=%.3f top_price_cents=%d top_unit=%q high_confidence_count=%d total_candidates=%d",
		query,
		top.ID,
		top.Name,
		top.Score,
		top.PriceCents,
		top.Unit,
		highConfidenceCount,
		len(products),
	)

	if highConfidenceCount == 0 {
		log.Printf("SearchProductMaterials: catalog query=%q has no high-confidence candidates; verify selected variants before drafting", query)
	}
}

func tokenizeForMatch(value string) map[string]struct{} {
	set := make(map[string]struct{})
	for _, token := range strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return r != '-' && r != '+' && r != '.' && r != '/' && r != 'x' && (r < '0' || r > '9') && (r < 'a' || r > 'z')
	}) {
		token = strings.TrimSpace(token)
		if len(token) < 2 {
			continue
		}
		set[token] = struct{}{}
	}
	return set
}

func extractDimensionTokens(value string) map[string]struct{} {
	tokens := make(map[string]struct{})
	for _, token := range strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return r != '-' && r != 'x' && r != '/' && r != '.' && (r < '0' || r > '9') && (r < 'a' || r > 'z')
	}) {
		token = strings.TrimSpace(token)
		if isDimensionToken(token) {
			tokens[token] = struct{}{}
		}
	}
	return tokens
}

func isDimensionToken(token string) bool {
	if token == "" {
		return false
	}

	hasDigit := false
	hasSeparator := false
	for _, r := range token {
		if r >= '0' && r <= '9' {
			hasDigit = true
		}
		if r == 'x' || r == '-' {
			hasSeparator = true
		}
	}

	return hasDigit && hasSeparator
}

// unitLookup is the set of recognised unit tokens for exact matching.
var unitLookup = map[string]bool{
	"m1": true, "m2": true, "m3": true,
	"stuk": true, "stuks": true,
	"liter": true, "l": true,
	"cm": true, "mm": true,
	"meter": true, "per": true,
}

func extractUnitTokens(value string) map[string]struct{} {
	units := map[string]struct{}{}
	tokens := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})
	for _, token := range tokens {
		if unitLookup[token] {
			units[token] = struct{}{}
		}
	}
	return units
}

func tokenOverlapRatio(queryTokens map[string]struct{}, textTokens map[string]struct{}) float64 {
	if len(queryTokens) == 0 {
		return 0
	}
	intersection := 0
	for token := range queryTokens {
		if _, ok := textTokens[token]; ok {
			intersection++
		}
	}
	return float64(intersection) / float64(len(queryTokens))
}

func countSetIntersection(a map[string]struct{}, b map[string]struct{}) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	count := 0
	for key := range a {
		if _, ok := b[key]; ok {
			count++
		}
	}
	return count
}

func hasAnyUnitToken(text string, units map[string]struct{}) bool {
	if len(units) == 0 {
		return false
	}
	tokens := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})
	for _, token := range tokens {
		if _, ok := units[token]; ok {
			return true
		}
	}
	return false
}

func extractProductFromPayload(payload map[string]any, score float64) ProductResult {
	product := ProductResult{Score: score}
	product.ID = payloadStr(payload, "id")
	product.Name = payloadStr(payload, "name")
	product.Description = payloadStr(payload, "description")
	product.Type = resolveProductType(payload)
	product.PriceEuros = payloadFloat(payload, "price")
	product.Unit = resolveUnit(payload)
	product.LaborTime = strings.TrimSpace(payloadStr(payload, "labor_time_text"))
	product.Category = payloadStr(payload, "category")
	product.SourceURL = payloadStr(payload, "source_url")

	if product.PriceEuros <= 0 {
		product.PriceEuros = payloadFloat(payload, "unit_price")
	}

	applyBrandPrefix(&product, payloadStr(payload, "brand"))
	extractSpecsMaterial(&product, payload)

	product.PriceCents = eurosToCents(product.PriceEuros)
	return product
}

// payloadStr safely extracts a string value from the payload map.
func payloadStr(payload map[string]any, key string) string {
	v, _ := payload[key].(string)
	return v
}

// payloadFloat safely extracts a float64 value from the payload map.
func payloadFloat(payload map[string]any, key string) float64 {
	v, _ := payload[key].(float64)
	return v
}

// resolveUnit determines the unit label from the payload, preferring
// unit_label > unit > parsed from price_raw.
func resolveUnit(payload map[string]any) string {
	if u := payloadStr(payload, "unit_label"); u != "" {
		return u
	}
	if u := payloadStr(payload, "unit"); u != "" {
		return u
	}
	return parseUnitFromPriceRaw(payload)
}

// resolveProductType returns the product type from the payload.
// Catalog products have a "type" field (service, digital_service, product, material).
// Fallback/scraped products default to "material".
func resolveProductType(payload map[string]any) string {
	if t := payloadStr(payload, "type"); t != "" {
		return t
	}
	return "material"
}

// applyBrandPrefix prepends the brand to the product description if present.
func applyBrandPrefix(product *ProductResult, brand string) {
	if brand == "" {
		return
	}
	if product.Description != "" {
		product.Description = brand + " — " + product.Description
	} else {
		product.Description = brand
	}
}

// parseUnitFromPriceRaw extracts a unit string from the scraped price_raw field.
// e.g. "€1,21/m1" → "per m1", "€3,50/stuk" → "per stuk".
func parseUnitFromPriceRaw(payload map[string]any) string {
	raw, ok := payload["price_raw"].(string)
	if !ok || raw == "" {
		return ""
	}
	idx := strings.LastIndex(raw, "/")
	if idx < 0 || idx >= len(raw)-1 {
		return ""
	}
	unit := strings.TrimSpace(raw[idx+1:])
	if unit == "" {
		return ""
	}
	return "per " + unit
}

// extractSpecsMaterial reads specs.raw.Materiaal from the payload and populates
// the product's Materials slice if it's empty.
func extractSpecsMaterial(product *ProductResult, payload map[string]any) {
	if len(product.Materials) > 0 {
		return
	}
	specs, ok := payload["specs"].(map[string]any)
	if !ok {
		return
	}
	raw, ok := specs["raw"].(map[string]any)
	if !ok {
		return
	}
	if mat, ok := raw["Materiaal"].(string); ok && mat != "" {
		product.Materials = []string{mat}
	}
}

// eurosToCents converts a euro amount to integer cents, rounding to nearest.
func eurosToCents(euros float64) int64 {
	return int64(math.Round(euros * 100))
}

// hydrateProductResults enriches vector-search ProductResults with DB-accurate
// pricing, VAT rates, and materials via the CatalogReader port. Products whose
// IDs cannot be resolved are returned unchanged.
func hydrateProductResults(ctx context.Context, deps *ToolDependencies, products []ProductResult) []ProductResult {
	if deps.CatalogReader == nil {
		return products
	}
	tenantID, ok := deps.GetTenantID()
	if !ok || tenantID == nil {
		return products
	}

	ids := collectProductUUIDs(products)
	if len(ids) == 0 {
		return products
	}

	hydrateCtx, hydrateCancel := detachedTimeout(ctx, toolIOTimeout)
	defer hydrateCancel()
	details, err := deps.CatalogReader.GetProductDetails(hydrateCtx, *tenantID, ids)
	if err != nil {
		log.Printf("hydrateProductResults: catalog reader failed: %v", err)
		return products
	}

	// Safety: only keep results that resolve to a non-draft catalog product.
	// The CatalogReader adapter omits unknown IDs and draft products.
	resolved := make(map[string]ports.CatalogProductDetails, len(details))
	for _, d := range details {
		resolved[d.ID.String()] = d
	}
	if len(resolved) == 0 {
		return nil
	}

	filtered := make([]ProductResult, 0, len(products))
	for _, p := range products {
		if p.ID == "" {
			continue
		}
		if _, ok := resolved[p.ID]; !ok {
			continue
		}
		filtered = append(filtered, p)
	}

	return applyProductDetails(filtered, details)
}

// collectProductUUIDs extracts unique, parseable UUIDs from product results.
func collectProductUUIDs(products []ProductResult) []uuid.UUID {
	seen := make(map[string]struct{}, len(products))
	ids := make([]uuid.UUID, 0, len(products))
	for _, p := range products {
		if p.ID == "" {
			continue
		}
		if _, dup := seen[p.ID]; dup {
			continue
		}
		uid, err := uuid.Parse(p.ID)
		if err != nil {
			continue
		}
		seen[p.ID] = struct{}{}
		ids = append(ids, uid)
	}
	return ids
}

// applyProductDetails merges DB-accurate catalog details back into product results.
func applyProductDetails(products []ProductResult, details []ports.CatalogProductDetails) []ProductResult {
	detailMap := make(map[string]ports.CatalogProductDetails, len(details))
	for _, d := range details {
		detailMap[d.ID.String()] = d
	}

	for i, p := range products {
		d, ok := detailMap[p.ID]
		if !ok {
			continue
		}
		products[i].PriceEuros = float64(d.UnitPriceCents) / 100
		products[i].PriceCents = d.UnitPriceCents
		products[i].VatRateBps = d.VatRateBps
		products[i].Materials = d.Materials
		mergeOptionalString(&products[i].Unit, d.UnitLabel)
		mergeOptionalString(&products[i].LaborTime, d.LaborTimeText)
		mergeOptionalString(&products[i].Description, d.Description)
	}
	return products
}

// mergeOptionalString overwrites dst when src is non-empty.
func mergeOptionalString(dst *string, src string) {
	if src != "" {
		*dst = src
	}
}

func createSearchProductMaterialsTool(_ *ToolDependencies) (tool.Tool, error) {
	return apptools.NewSearchProductMaterialsTool(func(ctx tool.Context, input SearchProductMaterialsInput) (SearchProductMaterialsOutput, error) {
		deps, err := GetDependencies(ctx)
		if err != nil {
			return SearchProductMaterialsOutput{}, err
		}
		return handleSearchProductMaterials(ctx, deps, input)
	})
}
