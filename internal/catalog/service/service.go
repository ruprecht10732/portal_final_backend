package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"portal_final_backend/internal/adapters/storage"
	"portal_final_backend/internal/catalog/repository"
	"portal_final_backend/internal/catalog/transport"
	"portal_final_backend/platform/ai/embeddingapi"
	"portal_final_backend/platform/apperr"
	"portal_final_backend/platform/logger"
)

const errPriceAndUnitPriceNonNegative = "priceCents and unitPriceCents must be 0 or greater"

// Service provides business logic for catalog.
type Service struct {
	repo                repository.Repository
	storage             storage.StorageService
	bucket              string
	log                 *logger.Logger
	embeddingClient     *embeddingapi.Client
	embeddingCollection string
}

// New creates a new catalog service.
func New(repo repository.Repository, storageSvc storage.StorageService, bucket string, log *logger.Logger, embeddingClient *embeddingapi.Client, embeddingCollection string) *Service {
	return &Service{
		repo:                repo,
		storage:             storageSvc,
		bucket:              bucket,
		log:                 log,
		embeddingClient:     embeddingClient,
		embeddingCollection: strings.TrimSpace(embeddingCollection),
	}
}

// GetVatRateByID retrieves a VAT rate by ID.
func (s *Service) GetVatRateByID(ctx context.Context, tenantID uuid.UUID, id uuid.UUID) (transport.VatRateResponse, error) {
	rate, err := s.repo.GetVatRateByID(ctx, tenantID, id)
	if err != nil {
		return transport.VatRateResponse{}, err
	}
	return toVatRateResponse(rate), nil
}

// ListVatRatesWithFilters retrieves VAT rates with search and pagination.
func (s *Service) ListVatRatesWithFilters(ctx context.Context, tenantID uuid.UUID, req transport.ListVatRatesRequest) (transport.VatRateListResponse, error) {
	page := req.Page
	pageSize := req.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	params := repository.ListVatRatesParams{
		OrganizationID: tenantID,
		Search:         strings.TrimSpace(req.Search),
		Offset:         (page - 1) * pageSize,
		Limit:          pageSize,
		SortBy:         req.SortBy,
		SortOrder:      req.SortOrder,
	}

	items, total, err := s.repo.ListVatRates(ctx, params)
	if err != nil {
		return transport.VatRateListResponse{}, err
	}

	return toVatRateListResponse(items, total, page, pageSize), nil
}

// CreateVatRate creates a new VAT rate.
func (s *Service) CreateVatRate(ctx context.Context, tenantID uuid.UUID, req transport.CreateVatRateRequest) (transport.VatRateResponse, error) {
	rate, err := s.repo.CreateVatRate(ctx, repository.CreateVatRateParams{
		OrganizationID: tenantID,
		Name:           strings.TrimSpace(req.Name),
		RateBps:        *req.RateBps,
	})
	if err != nil {
		return transport.VatRateResponse{}, err
	}

	s.log.Info("vat rate created", "id", rate.ID, "name", rate.Name)
	return toVatRateResponse(rate), nil
}

// UpdateVatRate updates an existing VAT rate.
func (s *Service) UpdateVatRate(ctx context.Context, tenantID uuid.UUID, id uuid.UUID, req transport.UpdateVatRateRequest) (transport.VatRateResponse, error) {
	name := req.Name
	if name != nil {
		trimmed := strings.TrimSpace(*name)
		name = &trimmed
	}

	rate, err := s.repo.UpdateVatRate(ctx, repository.UpdateVatRateParams{
		ID:             id,
		OrganizationID: tenantID,
		Name:           name,
		RateBps:        req.RateBps,
	})
	if err != nil {
		return transport.VatRateResponse{}, err
	}

	s.log.Info("vat rate updated", "id", rate.ID, "name", rate.Name)
	return toVatRateResponse(rate), nil
}

// DeleteVatRate deletes a VAT rate if not referenced by products.
func (s *Service) DeleteVatRate(ctx context.Context, tenantID uuid.UUID, id uuid.UUID) error {
	used, err := s.repo.HasProductsWithVatRate(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if used {
		return apperr.Conflict("vat rate is in use")
	}
	if err := s.repo.DeleteVatRate(ctx, tenantID, id); err != nil {
		return err
	}

	s.log.Info("vat rate deleted", "id", id)
	return nil
}

// SeedDefaultVatRates ensures a tenant has the standard VAT rates.
func (s *Service) SeedDefaultVatRates(ctx context.Context, tenantID uuid.UUID) error {
	items, total, err := s.repo.ListVatRates(ctx, repository.ListVatRatesParams{
		OrganizationID: tenantID,
		Offset:         0,
		Limit:          1,
	})
	if err != nil {
		return err
	}
	if total > 0 || len(items) > 0 {
		return nil
	}

	for _, def := range defaultVatRates {
		_, err := s.repo.CreateVatRate(ctx, repository.CreateVatRateParams{
			OrganizationID: tenantID,
			Name:           def.Name,
			RateBps:        def.RateBps,
		})
		if err != nil {
			return err
		}
	}

	return nil
}

// GetProductByID retrieves a product by ID.
func (s *Service) GetProductByID(ctx context.Context, tenantID uuid.UUID, id uuid.UUID) (transport.ProductResponse, error) {
	product, err := s.repo.GetProductByID(ctx, tenantID, id)
	if err != nil {
		return transport.ProductResponse{}, err
	}
	return toProductResponse(product), nil
}

// ListProductsWithFilters retrieves products with search and pagination.
func (s *Service) ListProductsWithFilters(ctx context.Context, tenantID uuid.UUID, req transport.ListProductsRequest, vatRateID *uuid.UUID) (transport.ProductListResponse, error) {
	page := req.Page
	pageSize := req.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	createdAtFrom, err := parseOptionalTime(req.CreatedAtFrom)
	if err != nil {
		return transport.ProductListResponse{}, err
	}
	createdAtTo, err := parseOptionalTime(req.CreatedAtTo)
	if err != nil {
		return transport.ProductListResponse{}, err
	}
	updatedAtFrom, err := parseOptionalTime(req.UpdatedAtFrom)
	if err != nil {
		return transport.ProductListResponse{}, err
	}
	updatedAtTo, err := parseOptionalTime(req.UpdatedAtTo)
	if err != nil {
		return transport.ProductListResponse{}, err
	}

	params := repository.ListProductsParams{
		OrganizationID: tenantID,
		Search:         strings.TrimSpace(req.Search),
		Title:          strings.TrimSpace(req.Title),
		Reference:      strings.TrimSpace(req.Reference),
		Type:           strings.TrimSpace(req.Type),
		IsDraft:        req.IsDraft,
		VatRateID:      vatRateID,
		CreatedAtFrom:  createdAtFrom,
		CreatedAtTo:    createdAtTo,
		UpdatedAtFrom:  updatedAtFrom,
		UpdatedAtTo:    updatedAtTo,
		Offset:         (page - 1) * pageSize,
		Limit:          pageSize,
		SortBy:         req.SortBy,
		SortOrder:      req.SortOrder,
	}

	items, total, err := s.repo.ListProducts(ctx, params)
	if err != nil {
		return transport.ProductListResponse{}, err
	}

	return toProductListResponse(items, total, page, pageSize), nil
}

func parseOptionalTime(value string) (*time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}
	if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return &parsed, nil
	}
	if parsed, err := time.Parse("2006-01-02", trimmed); err == nil {
		return &parsed, nil
	}
	return nil, apperr.Validation("invalid date format")
}

// CreateProduct creates a new product.
func (s *Service) CreateProduct(ctx context.Context, tenantID uuid.UUID, req transport.CreateProductRequest) (transport.ProductResponse, error) {
	if err := s.validatePeriod(req.PeriodCount, req.PeriodUnit); err != nil {
		return transport.ProductResponse{}, err
	}
	unitLabel, err := s.validatePricingCreate(req.PriceCents, req.UnitPriceCents, req.UnitLabel)
	if err != nil {
		return transport.ProductResponse{}, err
	}
	if _, err := s.repo.GetVatRateByID(ctx, tenantID, req.VatRateID); err != nil {
		return transport.ProductResponse{}, err
	}

	reference := strings.TrimSpace(req.Reference)
	if reference == "" {
		reference, err = s.repo.NextProductReference(ctx, tenantID)
		if err != nil {
			return transport.ProductResponse{}, err
		}
	}

	isDraft := false
	if req.IsDraft != nil {
		isDraft = *req.IsDraft
	}

	product, err := s.repo.CreateProduct(ctx, repository.CreateProductParams{
		OrganizationID: tenantID,
		VatRateID:      req.VatRateID,
		IsDraft:        isDraft,
		Title:          strings.TrimSpace(req.Title),
		Reference:      reference,
		Description:    trimPtr(req.Description),
		PriceCents:     req.PriceCents,
		UnitPriceCents: req.UnitPriceCents,
		UnitLabel:      unitLabel,
		LaborTimeText:  trimPtr(req.LaborTimeText),
		Type:           req.Type,
		PeriodCount:    req.PeriodCount,
		PeriodUnit:     req.PeriodUnit,
	})
	if err != nil {
		return transport.ProductResponse{}, err
	}

	s.log.Info("product created", "id", product.ID, "reference", product.Reference)
	s.indexProductAsync(ctx, tenantID, product, "create")
	return toProductResponse(product), nil
}

// GetNextProductReference retrieves the next auto-generated product reference for pre-filling create forms.
func (s *Service) GetNextProductReference(ctx context.Context, tenantID uuid.UUID) (transport.NextProductReferenceResponse, error) {
	reference, err := s.repo.NextProductReference(ctx, tenantID)
	if err != nil {
		return transport.NextProductReferenceResponse{}, err
	}

	return transport.NextProductReferenceResponse{Reference: reference}, nil
}

// UpdateProduct updates an existing product.
func (s *Service) UpdateProduct(ctx context.Context, tenantID uuid.UUID, id uuid.UUID, req transport.UpdateProductRequest) (transport.ProductResponse, error) {
	if err := s.ensureVatRateExists(ctx, tenantID, req.VatRateID); err != nil {
		return transport.ProductResponse{}, err
	}
	unitLabel, err := s.validatePricingUpdate(req.PriceCents, req.UnitPriceCents, req.UnitLabel)
	if err != nil {
		return transport.ProductResponse{}, err
	}
	if err := s.validatePeriodUpdate(req); err != nil {
		return transport.ProductResponse{}, err
	}
	if err := s.ensureTypeChangeAllowed(ctx, tenantID, id, req.Type); err != nil {
		return transport.ProductResponse{}, err
	}

	params := repository.UpdateProductParams{
		ID:             id,
		OrganizationID: tenantID,
		VatRateID:      req.VatRateID,
		IsDraft:        req.IsDraft,
		Title:          trimPtr(req.Title),
		Reference:      trimPtr(req.Reference),
		Description:    trimPtr(req.Description),
		PriceCents:     req.PriceCents,
		UnitPriceCents: req.UnitPriceCents,
		UnitLabel:      unitLabel,
		LaborTimeText:  trimPtr(req.LaborTimeText),
		Type:           req.Type,
		PeriodCount:    req.PeriodCount,
		PeriodUnit:     req.PeriodUnit,
	}

	product, err := s.repo.UpdateProduct(ctx, params)
	if err != nil {
		return transport.ProductResponse{}, err
	}

	s.log.Info("product updated", "id", product.ID, "reference", product.Reference)
	s.indexProductAsync(ctx, tenantID, product, "update")
	return toProductResponse(product), nil
}

func (s *Service) indexProductAsync(ctx context.Context, tenantID uuid.UUID, product repository.Product, reason string) {
	if s.embeddingClient == nil {
		return
	}

	request := embeddingapi.AddDocumentsRequest{
		Documents:  []map[string]any{s.buildCatalogDocument(tenantID, product)},
		TextFields: []string{"name", "description", "reference", "type", "labor_time_text", "unit_label"},
		IDField:    "id",
		Collection: s.embeddingCollection,
	}

	loggerWithCtx := s.log.WithContext(ctx)
	go func() {
		resp, err := s.embeddingClient.AddDocuments(context.Background(), request)
		if err != nil {
			loggerWithCtx.Error("catalog indexing failed", "error", err, "productId", product.ID, "reason", reason)
			return
		}
		loggerWithCtx.Info("catalog indexed", "productId", product.ID, "documentsAdded", resp.DocumentsAdded, "reason", reason)
	}()
}

func (s *Service) buildCatalogDocument(tenantID uuid.UUID, product repository.Product) map[string]any {
	document := map[string]any{
		"id":               product.ID.String(),
		"organization_id":  tenantID.String(),
		"name":             product.Title,
		"reference":        product.Reference,
		"type":             product.Type,
		"price":            float64(product.PriceCents) / 100,
		"price_cents":      product.PriceCents,
		"unit_price":       float64(product.UnitPriceCents) / 100,
		"unit_price_cents": product.UnitPriceCents,
		"vat_rate_id":      product.VatRateID.String(),
	}
	if product.UnitLabel != nil && strings.TrimSpace(*product.UnitLabel) != "" {
		document["unit_label"] = strings.TrimSpace(*product.UnitLabel)
	}
	if product.LaborTimeText != nil {
		trimmed := strings.TrimSpace(*product.LaborTimeText)
		if trimmed != "" {
			document["labor_time_text"] = trimmed
		}
	}

	if product.Description != nil {
		trimmed := strings.TrimSpace(*product.Description)
		if trimmed != "" {
			document["description"] = trimmed
		}
	}
	if product.PeriodCount != nil {
		document["period_count"] = *product.PeriodCount
	}
	if product.PeriodUnit != nil {
		document["period_unit"] = *product.PeriodUnit
	}

	return document
}

// DeleteProduct deletes a product.
func (s *Service) DeleteProduct(ctx context.Context, tenantID uuid.UUID, id uuid.UUID) error {
	if err := s.repo.DeleteProduct(ctx, tenantID, id); err != nil {
		return err
	}
	s.log.Info("product deleted", "id", id)
	return nil
}

// AddProductMaterials adds material products to a service product.
func (s *Service) AddProductMaterials(ctx context.Context, tenantID uuid.UUID, productID uuid.UUID, links []repository.ProductMaterialLink) error {
	if err := s.ensureServiceProduct(ctx, tenantID, productID); err != nil {
		return err
	}
	uniqueIDs, normalizedLinks, err := s.ensureValidMaterialLinks(productID, links)
	if err != nil {
		return err
	}
	materials, err := s.loadAndValidateMaterials(ctx, tenantID, uniqueIDs)
	if err != nil {
		return err
	}
	if err := s.ensureMaterialsNoChildren(ctx, tenantID, materials); err != nil {
		return err
	}

	if err := s.repo.AddProductMaterials(ctx, tenantID, productID, normalizedLinks); err != nil {
		return err
	}

	s.log.Info("product materials added", "productId", productID, "count", len(uniqueIDs))
	return nil
}

// RemoveProductMaterials removes materials from a product.
func (s *Service) RemoveProductMaterials(ctx context.Context, tenantID uuid.UUID, productID uuid.UUID, materialIDs []uuid.UUID) error {
	uniqueIDs := uniqueUUIDs(materialIDs)
	if err := s.repo.RemoveProductMaterials(ctx, tenantID, productID, uniqueIDs); err != nil {
		return err
	}

	s.log.Info("product materials removed", "productId", productID, "count", len(uniqueIDs))
	return nil
}

func (s *Service) ensureVatRateExists(ctx context.Context, tenantID uuid.UUID, vatRateID *uuid.UUID) error {
	if vatRateID == nil {
		return nil
	}
	_, err := s.repo.GetVatRateByID(ctx, tenantID, *vatRateID)
	return err
}

func (s *Service) validatePeriodUpdate(req transport.UpdateProductRequest) error {
	if req.PeriodCount == nil && req.PeriodUnit == nil {
		return nil
	}
	return s.validatePeriod(req.PeriodCount, req.PeriodUnit)
}

func (s *Service) ensureTypeChangeAllowed(ctx context.Context, tenantID uuid.UUID, productID uuid.UUID, productType *string) error {
	if productType == nil || *productType == "service" {
		return nil
	}
	hasMaterials, err := s.repo.HasProductMaterials(ctx, tenantID, productID)
	if err != nil {
		return err
	}
	if hasMaterials {
		return apperr.Conflict("product has materials and cannot change type")
	}
	return nil
}

func (s *Service) ensureServiceProduct(ctx context.Context, tenantID uuid.UUID, productID uuid.UUID) error {
	product, err := s.repo.GetProductByID(ctx, tenantID, productID)
	if err != nil {
		return err
	}
	if product.Type != "service" {
		return apperr.Validation("materials can only be linked to service products")
	}
	return nil
}

func (s *Service) ensureValidMaterialLinks(productID uuid.UUID, links []repository.ProductMaterialLink) ([]uuid.UUID, []repository.ProductMaterialLink, error) {
	if len(links) == 0 {
		return nil, nil, apperr.Validation("at least one material is required")
	}

	seen := make(map[uuid.UUID]struct{}, len(links))
	materialIDs := make([]uuid.UUID, 0, len(links))
	normalizedLinks := make([]repository.ProductMaterialLink, 0, len(links))

	for _, link := range links {
		if link.MaterialID == productID {
			return nil, nil, apperr.Validation("product cannot reference itself as a material")
		}
		if !isAllowedPricingMode(link.PricingMode) {
			return nil, nil, apperr.Validation("invalid pricingMode")
		}
		if _, exists := seen[link.MaterialID]; exists {
			continue
		}
		seen[link.MaterialID] = struct{}{}
		materialIDs = append(materialIDs, link.MaterialID)
		normalizedLinks = append(normalizedLinks, repository.ProductMaterialLink{
			MaterialID:  link.MaterialID,
			PricingMode: link.PricingMode,
		})
	}

	return materialIDs, normalizedLinks, nil
}

func (s *Service) loadAndValidateMaterials(ctx context.Context, tenantID uuid.UUID, materialIDs []uuid.UUID) ([]repository.Product, error) {
	materials, err := s.repo.GetProductsByIDs(ctx, tenantID, materialIDs)
	if err != nil {
		return nil, err
	}
	if len(materials) != len(materialIDs) {
		return nil, apperr.Validation("one or more materials were not found")
	}
	for _, material := range materials {
		if material.Type != "material" {
			return nil, apperr.Validation("only material products can be linked")
		}
	}
	return materials, nil
}

func (s *Service) ensureMaterialsNoChildren(ctx context.Context, tenantID uuid.UUID, materials []repository.Product) error {
	// Defense-in-depth: verify materials don't have their own children.
	// The type system already prevents this (only "material" types can be added,
	// and "material" types cannot have children), but this check ensures
	// future changes don't accidentally create circular dependencies.
	for _, material := range materials {
		hasChildren, err := s.repo.HasProductMaterials(ctx, tenantID, material.ID)
		if err != nil {
			return err
		}
		if hasChildren {
			return apperr.Validation("cannot add a material that is composed of other materials")
		}
	}
	return nil
}

// ListProductMaterials lists materials linked to a product.
func (s *Service) ListProductMaterials(ctx context.Context, tenantID uuid.UUID, productID uuid.UUID) ([]transport.ProductResponse, error) {
	items, err := s.repo.ListProductMaterials(ctx, tenantID, productID)
	if err != nil {
		return nil, err
	}

	responses := make([]transport.ProductResponse, len(items))
	for i, item := range items {
		responses[i] = toProductResponse(item)
	}

	return responses, nil
}

func (s *Service) validatePeriod(count *int, unit *string) error {
	if count == nil && unit == nil {
		return nil
	}
	if count == nil || unit == nil {
		return apperr.Validation("periodCount and periodUnit must be provided together")
	}
	if *count <= 0 {
		return apperr.Validation("periodCount must be greater than 0")
	}
	if !isAllowedPeriodUnit(*unit) {
		return apperr.Validation("invalid periodUnit")
	}
	return nil
}

func isAllowedPeriodUnit(unit string) bool {
	switch unit {
	case "day", "week", "month", "quarter", "year":
		return true
	default:
		return false
	}
}

func isAllowedPricingMode(mode string) bool {
	switch mode {
	case "included", "additional", "optional":
		return true
	default:
		return false
	}
}

func trimPtr(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	return &trimmed
}

func uniqueUUIDs(values []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(values))
	result := make([]uuid.UUID, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func toVatRateResponse(rate repository.VatRate) transport.VatRateResponse {
	return transport.VatRateResponse{
		ID:        rate.ID,
		Name:      rate.Name,
		RateBps:   rate.RateBps,
		CreatedAt: rate.CreatedAt,
		UpdatedAt: rate.UpdatedAt,
	}
}

func toVatRateListResponse(items []repository.VatRate, total int, page int, pageSize int) transport.VatRateListResponse {
	responses := make([]transport.VatRateResponse, len(items))
	for i, item := range items {
		responses[i] = toVatRateResponse(item)
	}
	if pageSize < 1 {
		pageSize = len(items)
	}
	totalPages := 0
	if pageSize > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}
	return transport.VatRateListResponse{
		Items:      responses,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}
}

// SearchForAutocomplete returns a lightweight list of products with their document
// and URL assets for use in quote-line ghost-text autocomplete.
func (s *Service) SearchForAutocomplete(ctx context.Context, tenantID uuid.UUID, req transport.AutocompleteSearchRequest) ([]transport.AutocompleteItemResponse, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 5
	}

	products, _, err := s.repo.ListProducts(ctx, repository.ListProductsParams{
		OrganizationID: tenantID,
		Search:         strings.TrimSpace(req.Query),
		Limit:          limit,
		Offset:         0,
		SortBy:         "title",
		SortOrder:      "asc",
	})
	if err != nil {
		return nil, fmt.Errorf("search products: %w", err)
	}

	result := make([]transport.AutocompleteItemResponse, 0, len(products))
	for _, p := range products {
		item, err := s.buildAutocompleteItem(ctx, tenantID, p)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}

	return result, nil
}

func (s *Service) buildAutocompleteItem(ctx context.Context, tenantID uuid.UUID, p repository.Product) (transport.AutocompleteItemResponse, error) {
	docs, err := s.repo.ListProductAssets(ctx, repository.ListProductAssetsParams{
		OrganizationID: tenantID,
		ProductID:      p.ID,
		AssetType:      strPtr("document"),
	})
	if err != nil {
		return transport.AutocompleteItemResponse{}, fmt.Errorf("list product assets: %w", err)
	}

	urls, err := s.repo.ListProductAssets(ctx, repository.ListProductAssetsParams{
		OrganizationID: tenantID,
		ProductID:      p.ID,
		AssetType:      strPtr("terms_url"),
	})
	if err != nil {
		return transport.AutocompleteItemResponse{}, fmt.Errorf("list product url assets: %w", err)
	}

	var vatRateBps int
	rate, err := s.repo.GetVatRateByID(ctx, tenantID, p.VatRateID)
	if err == nil {
		vatRateBps = rate.RateBps
	}

	return transport.AutocompleteItemResponse{
		ID:             p.ID,
		Title:          p.Title,
		Description:    p.Description,
		PriceCents:     p.PriceCents,
		UnitPriceCents: p.UnitPriceCents,
		UnitLabel:      p.UnitLabel,
		VatRateID:      p.VatRateID,
		VatRateBps:     vatRateBps,
		Documents:      toAutocompleteDocuments(docs),
		URLs:           toAutocompleteURLs(urls),
	}, nil
}

func strPtr(s string) *string { return &s }

func toAutocompleteDocuments(assets []repository.ProductAsset) []transport.AutocompleteDocumentResponse {
	out := make([]transport.AutocompleteDocumentResponse, 0, len(assets))
	for _, d := range assets {
		if d.FileKey != nil && d.FileName != nil {
			out = append(out, transport.AutocompleteDocumentResponse{
				ID:       d.ID,
				Filename: *d.FileName,
				FileKey:  *d.FileKey,
			})
		}
	}
	return out
}

func toAutocompleteURLs(assets []repository.ProductAsset) []transport.AutocompleteURLResponse {
	out := make([]transport.AutocompleteURLResponse, 0, len(assets))
	for _, u := range assets {
		if u.URL == nil {
			continue
		}
		label := "Voorwaarden"
		if u.FileName != nil {
			label = *u.FileName
		}
		out = append(out, transport.AutocompleteURLResponse{
			Label: label,
			Href:  *u.URL,
		})
	}
	return out
}

func toProductResponse(product repository.Product) transport.ProductResponse {
	return transport.ProductResponse{
		ID:             product.ID,
		VatRateID:      product.VatRateID,
		IsDraft:        product.IsDraft,
		Title:          product.Title,
		Reference:      product.Reference,
		Description:    product.Description,
		PriceCents:     product.PriceCents,
		UnitPriceCents: product.UnitPriceCents,
		UnitLabel:      product.UnitLabel,
		LaborTimeText:  product.LaborTimeText,
		Type:           product.Type,
		PricingMode:    product.PricingMode,
		PeriodCount:    product.PeriodCount,
		PeriodUnit:     product.PeriodUnit,
		CreatedAt:      product.CreatedAt,
		UpdatedAt:      product.UpdatedAt,
	}
}

func (s *Service) validatePricingCreate(priceCents int64, unitPriceCents int64, unitLabel *string) (*string, error) {
	trimmed := trimPtr(unitLabel)
	if priceCents < 0 || unitPriceCents < 0 {
		return nil, apperr.Validation(errPriceAndUnitPriceNonNegative)
	}
	if priceCents > 0 && unitPriceCents > 0 {
		return nil, apperr.Validation("choose either priceCents or unitPriceCents")
	}
	if unitPriceCents > 0 && (trimmed == nil || *trimmed == "") {
		return nil, apperr.Validation("unitLabel is required when unitPriceCents is set")
	}
	return trimmed, nil
}

func (s *Service) validatePricingUpdate(priceCents *int64, unitPriceCents *int64, unitLabel *string) (*string, error) {
	trimmed := trimPtr(unitLabel)
	if priceCents == nil && unitPriceCents == nil {
		return trimmed, nil
	}
	price := int64(0)
	unitPrice := int64(0)
	if priceCents != nil {
		price = *priceCents
		if price < 0 {
			return nil, apperr.Validation(errPriceAndUnitPriceNonNegative)
		}
	}
	if unitPriceCents != nil {
		unitPrice = *unitPriceCents
		if unitPrice < 0 {
			return nil, apperr.Validation(errPriceAndUnitPriceNonNegative)
		}
	}
	if price > 0 && unitPrice > 0 {
		return nil, apperr.Validation("choose either priceCents or unitPriceCents")
	}
	if unitPrice > 0 && (trimmed == nil || *trimmed == "") {
		return nil, apperr.Validation("unitLabel is required when unitPriceCents is set")
	}
	return trimmed, nil
}

func toProductListResponse(items []repository.Product, total int, page int, pageSize int) transport.ProductListResponse {
	responses := make([]transport.ProductResponse, len(items))
	for i, item := range items {
		responses[i] = toProductResponse(item)
	}
	if pageSize < 1 {
		pageSize = len(items)
	}
	totalPages := 0
	if pageSize > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}
	return transport.ProductListResponse{
		Items:      responses,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}
}

// Asset operations

func (s *Service) GetCatalogAssetPresign(ctx context.Context, tenantID uuid.UUID, productID uuid.UUID, req transport.PresignCatalogAssetRequest) (transport.PresignedUploadResponse, error) {
	if _, err := s.repo.GetProductByID(ctx, tenantID, productID); err != nil {
		return transport.PresignedUploadResponse{}, err
	}

	if err := s.storage.ValidateContentType(req.ContentType); err != nil {
		return transport.PresignedUploadResponse{}, apperr.Validation("file type not allowed")
	}
	if err := s.storage.ValidateFileSize(req.SizeBytes); err != nil {
		return transport.PresignedUploadResponse{}, apperr.Validation(err.Error())
	}
	if err := validateAssetType(req.AssetType, req.ContentType); err != nil {
		return transport.PresignedUploadResponse{}, err
	}

	folder := fmt.Sprintf("%s/%s/%s", tenantID.String(), productID.String(), req.AssetType)
	presigned, err := s.storage.GenerateUploadURL(ctx, s.bucket, folder, req.FileName, req.ContentType, req.SizeBytes)
	if err != nil {
		return transport.PresignedUploadResponse{}, err
	}

	return transport.PresignedUploadResponse{
		UploadURL: presigned.URL,
		FileKey:   presigned.FileKey,
		ExpiresAt: presigned.ExpiresAt.Unix(),
	}, nil
}

func (s *Service) CreateCatalogAsset(ctx context.Context, tenantID uuid.UUID, productID uuid.UUID, req transport.CreateCatalogAssetRequest) (transport.CatalogAssetResponse, error) {
	if _, err := s.repo.GetProductByID(ctx, tenantID, productID); err != nil {
		return transport.CatalogAssetResponse{}, err
	}

	if err := s.storage.ValidateContentType(req.ContentType); err != nil {
		return transport.CatalogAssetResponse{}, apperr.Validation("file type not allowed")
	}
	if err := s.storage.ValidateFileSize(req.SizeBytes); err != nil {
		return transport.CatalogAssetResponse{}, apperr.Validation(err.Error())
	}

	if err := validateAssetType(req.AssetType, req.ContentType); err != nil {
		return transport.CatalogAssetResponse{}, err
	}

	fileKey := strings.TrimSpace(req.FileKey)
	fileName := strings.TrimSpace(req.FileName)
	contentType := strings.TrimSpace(req.ContentType)
	sizeBytes := req.SizeBytes

	asset, err := s.repo.CreateProductAsset(ctx, repository.CreateProductAssetParams{
		OrganizationID: tenantID,
		ProductID:      productID,
		AssetType:      req.AssetType,
		FileKey:        &fileKey,
		FileName:       &fileName,
		ContentType:    &contentType,
		SizeBytes:      &sizeBytes,
		URL:            nil,
	})
	if err != nil {
		return transport.CatalogAssetResponse{}, err
	}

	s.log.Info("catalog asset created", "productId", productID, "assetId", asset.ID, "type", asset.AssetType)
	return toCatalogAssetResponse(asset), nil
}

func (s *Service) CreateCatalogURLAsset(ctx context.Context, tenantID uuid.UUID, productID uuid.UUID, req transport.CreateCatalogURLAssetRequest) (transport.CatalogAssetResponse, error) {
	if _, err := s.repo.GetProductByID(ctx, tenantID, productID); err != nil {
		return transport.CatalogAssetResponse{}, err
	}

	if req.AssetType != "terms_url" {
		return transport.CatalogAssetResponse{}, apperr.Validation("invalid assetType")
	}

	url := strings.TrimSpace(req.URL)
	var label *string
	if req.Label != nil {
		trimmed := strings.TrimSpace(*req.Label)
		label = &trimmed
	}

	asset, err := s.repo.CreateProductAsset(ctx, repository.CreateProductAssetParams{
		OrganizationID: tenantID,
		ProductID:      productID,
		AssetType:      req.AssetType,
		FileName:       label,
		URL:            &url,
	})
	if err != nil {
		return transport.CatalogAssetResponse{}, err
	}

	s.log.Info("catalog url asset created", "productId", productID, "assetId", asset.ID)
	return toCatalogAssetResponse(asset), nil
}

func (s *Service) ListCatalogAssets(ctx context.Context, tenantID uuid.UUID, productID uuid.UUID, assetType *string) (transport.CatalogAssetListResponse, error) {
	if _, err := s.repo.GetProductByID(ctx, tenantID, productID); err != nil {
		return transport.CatalogAssetListResponse{}, err
	}

	items, err := s.repo.ListProductAssets(ctx, repository.ListProductAssetsParams{
		OrganizationID: tenantID,
		ProductID:      productID,
		AssetType:      assetType,
	})
	if err != nil {
		return transport.CatalogAssetListResponse{}, err
	}

	responses := make([]transport.CatalogAssetResponse, len(items))
	for i, item := range items {
		responses[i] = toCatalogAssetResponse(item)
	}

	return transport.CatalogAssetListResponse{Items: responses}, nil
}

func (s *Service) GetCatalogAssetDownloadURL(ctx context.Context, tenantID uuid.UUID, productID uuid.UUID, assetID uuid.UUID) (transport.PresignedDownloadResponse, error) {
	asset, err := s.repo.GetProductAssetByID(ctx, tenantID, assetID)
	if err != nil {
		return transport.PresignedDownloadResponse{}, err
	}
	if asset.ProductID != productID {
		return transport.PresignedDownloadResponse{}, apperr.NotFound("product asset not found")
	}

	if asset.URL != nil {
		return transport.PresignedDownloadResponse{DownloadURL: *asset.URL}, nil
	}
	if asset.FileKey == nil {
		return transport.PresignedDownloadResponse{}, apperr.Validation("missing file key")
	}

	presigned, err := s.storage.GenerateDownloadURL(ctx, s.bucket, *asset.FileKey)
	if err != nil {
		return transport.PresignedDownloadResponse{}, err
	}
	expiresAt := presigned.ExpiresAt.Unix()

	return transport.PresignedDownloadResponse{
		DownloadURL: presigned.URL,
		ExpiresAt:   &expiresAt,
	}, nil
}

func (s *Service) DeleteCatalogAsset(ctx context.Context, tenantID uuid.UUID, productID uuid.UUID, assetID uuid.UUID) error {
	asset, err := s.repo.GetProductAssetByID(ctx, tenantID, assetID)
	if err != nil {
		return err
	}
	if asset.ProductID != productID {
		return apperr.NotFound("product asset not found")
	}

	if asset.FileKey != nil {
		if err := s.storage.DeleteObject(ctx, s.bucket, *asset.FileKey); err != nil {
			return err
		}
	}

	if err := s.repo.DeleteProductAsset(ctx, tenantID, assetID); err != nil {
		return err
	}

	s.log.Info("catalog asset deleted", "productId", productID, "assetId", assetID)
	return nil
}

func validateAssetType(assetType string, contentType string) error {
	normalized := strings.TrimSpace(strings.Split(contentType, ";")[0])
	switch assetType {
	case "image":
		if !storage.IsImageContentType(normalized) {
			return apperr.Validation("assetType image requires image content type")
		}
	case "document":
		if !storage.IsDocumentContentType(normalized) {
			return apperr.Validation("assetType document requires document content type")
		}
	default:
		return apperr.Validation("invalid assetType")
	}
	return nil
}

func toCatalogAssetResponse(asset repository.ProductAsset) transport.CatalogAssetResponse {
	return transport.CatalogAssetResponse{
		ID:          asset.ID,
		ProductID:   asset.ProductID,
		AssetType:   asset.AssetType,
		FileKey:     asset.FileKey,
		FileName:    asset.FileName,
		ContentType: asset.ContentType,
		SizeBytes:   asset.SizeBytes,
		URL:         asset.URL,
		CreatedAt:   asset.CreatedAt,
	}
}

type defaultVatRate struct {
	Name    string
	RateBps int
}

var defaultVatRates = []defaultVatRate{
	{Name: "BTW 21%", RateBps: 2100},
	{Name: "BTW 9%", RateBps: 900},
	{Name: "BTW 0%", RateBps: 0},
}
