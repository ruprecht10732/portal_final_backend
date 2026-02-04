package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"portal_final_backend/internal/adapters/storage"
	"portal_final_backend/internal/catalog/repository"
	"portal_final_backend/internal/catalog/transport"
	"portal_final_backend/platform/apperr"
	"portal_final_backend/platform/logger"
	"portal_final_backend/platform/sanitize"
)

// Service provides business logic for catalog.
type Service struct {
	repo    repository.Repository
	storage storage.StorageService
	bucket  string
	log     *logger.Logger
}

// New creates a new catalog service.
func New(repo repository.Repository, storageSvc storage.StorageService, bucket string, log *logger.Logger) *Service {
	return &Service{repo: repo, storage: storageSvc, bucket: bucket, log: log}
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
		Description:    sanitize.TextPtr(req.Description),
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
	if err := s.ensureVatRateExists(ctx, tenantID, req.VatRateID); err != nil {
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
		Title:          trimPtr(req.Title),
		Reference:      trimPtr(req.Reference),
		Description:    sanitize.TextPtr(req.Description),
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
	if err := s.ensureServiceProduct(ctx, tenantID, productID); err != nil {
		return err
	}
	uniqueIDs, err := s.ensureValidMaterialIDs(productID, materialIDs)
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

func (s *Service) ensureValidMaterialIDs(productID uuid.UUID, materialIDs []uuid.UUID) ([]uuid.UUID, error) {
	uniqueIDs := uniqueUUIDs(materialIDs)
	for _, id := range uniqueIDs {
		if id == productID {
			return nil, apperr.Validation("product cannot reference itself as a material")
		}
	}
	return uniqueIDs, nil
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
