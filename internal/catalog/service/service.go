package service

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"portal_final_backend/internal/catalog/repository"
	"portal_final_backend/internal/catalog/transport"
	"portal_final_backend/platform/apperr"
	"portal_final_backend/platform/logger"
)

// Service provides business logic for catalog.
type Service struct {
	repo repository.Repository
	log  *logger.Logger
}

// New creates a new catalog service.
func New(repo repository.Repository, log *logger.Logger) *Service {
	return &Service{repo: repo, log: log}
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
		RateBps:        req.RateBps,
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

	params := repository.ListProductsParams{
		OrganizationID: tenantID,
		Search:         strings.TrimSpace(req.Search),
		Type:           strings.TrimSpace(req.Type),
		VatRateID:      vatRateID,
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

// CreateProduct creates a new product.
func (s *Service) CreateProduct(ctx context.Context, tenantID uuid.UUID, req transport.CreateProductRequest) (transport.ProductResponse, error) {
	if err := s.validatePeriod(req.PeriodCount, req.PeriodUnit); err != nil {
		return transport.ProductResponse{}, err
	}
	if _, err := s.repo.GetVatRateByID(ctx, tenantID, req.VatRateID); err != nil {
		return transport.ProductResponse{}, err
	}

	product, err := s.repo.CreateProduct(ctx, repository.CreateProductParams{
		OrganizationID: tenantID,
		VatRateID:      req.VatRateID,
		Title:          strings.TrimSpace(req.Title),
		Reference:      strings.TrimSpace(req.Reference),
		Description:    req.Description,
		PriceCents:     req.PriceCents,
		Type:           req.Type,
		PeriodCount:    req.PeriodCount,
		PeriodUnit:     req.PeriodUnit,
	})
	if err != nil {
		return transport.ProductResponse{}, err
	}

	s.log.Info("product created", "id", product.ID, "reference", product.Reference)
	return toProductResponse(product), nil
}

// UpdateProduct updates an existing product.
func (s *Service) UpdateProduct(ctx context.Context, tenantID uuid.UUID, id uuid.UUID, req transport.UpdateProductRequest) (transport.ProductResponse, error) {
	if req.VatRateID != nil {
		if _, err := s.repo.GetVatRateByID(ctx, tenantID, *req.VatRateID); err != nil {
			return transport.ProductResponse{}, err
		}
	}

	if req.PeriodCount != nil || req.PeriodUnit != nil {
		if err := s.validatePeriod(req.PeriodCount, req.PeriodUnit); err != nil {
			return transport.ProductResponse{}, err
		}
	}

	if req.Type != nil {
		if *req.Type != "service" {
			hasMaterials, err := s.repo.HasProductMaterials(ctx, tenantID, id)
			if err != nil {
				return transport.ProductResponse{}, err
			}
			if hasMaterials {
				return transport.ProductResponse{}, apperr.Conflict("product has materials and cannot change type")
			}
		}
	}

	params := repository.UpdateProductParams{
		ID:             id,
		OrganizationID: tenantID,
		VatRateID:      req.VatRateID,
		Title:          trimPtr(req.Title),
		Reference:      trimPtr(req.Reference),
		Description:    req.Description,
		PriceCents:     req.PriceCents,
		Type:           req.Type,
		PeriodCount:    req.PeriodCount,
		PeriodUnit:     req.PeriodUnit,
	}

	product, err := s.repo.UpdateProduct(ctx, params)
	if err != nil {
		return transport.ProductResponse{}, err
	}

	s.log.Info("product updated", "id", product.ID, "reference", product.Reference)
	return toProductResponse(product), nil
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
func (s *Service) AddProductMaterials(ctx context.Context, tenantID uuid.UUID, productID uuid.UUID, materialIDs []uuid.UUID) error {
	product, err := s.repo.GetProductByID(ctx, tenantID, productID)
	if err != nil {
		return err
	}
	if product.Type != "service" {
		return apperr.Validation("materials can only be linked to service products")
	}

	uniqueIDs := uniqueUUIDs(materialIDs)
	for _, id := range uniqueIDs {
		if id == productID {
			return apperr.Validation("product cannot reference itself as a material")
		}
	}

	materials, err := s.repo.GetProductsByIDs(ctx, tenantID, uniqueIDs)
	if err != nil {
		return err
	}
	if len(materials) != len(uniqueIDs) {
		return apperr.Validation("one or more materials were not found")
	}

	for _, material := range materials {
		if material.Type != "material" {
			return apperr.Validation("only material products can be linked")
		}
	}

	if err := s.repo.AddProductMaterials(ctx, tenantID, productID, uniqueIDs); err != nil {
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

func toProductResponse(product repository.Product) transport.ProductResponse {
	return transport.ProductResponse{
		ID:          product.ID,
		VatRateID:   product.VatRateID,
		Title:       product.Title,
		Reference:   product.Reference,
		Description: product.Description,
		PriceCents:  product.PriceCents,
		Type:        product.Type,
		PeriodCount: product.PeriodCount,
		PeriodUnit:  product.PeriodUnit,
		CreatedAt:   product.CreatedAt,
		UpdatedAt:   product.UpdatedAt,
	}
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
