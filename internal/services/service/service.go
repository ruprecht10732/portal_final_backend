package service

import (
	"context"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"portal_final_backend/internal/services/repository"
	"portal_final_backend/internal/services/transport"
	"portal_final_backend/platform/logger"
)

// Service provides business logic for service types.
type Service struct {
	repo repository.Repository
	log  *logger.Logger
}

// New creates a new service types service.
func New(repo repository.Repository, log *logger.Logger) *Service {
	return &Service{repo: repo, log: log}
}

// GetByID retrieves a service type by ID.
func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (transport.ServiceTypeResponse, error) {
	st, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return transport.ServiceTypeResponse{}, err
	}
	return toResponse(st), nil
}

// GetBySlug retrieves a service type by slug.
func (s *Service) GetBySlug(ctx context.Context, slug string) (transport.ServiceTypeResponse, error) {
	st, err := s.repo.GetBySlug(ctx, slug)
	if err != nil {
		return transport.ServiceTypeResponse{}, err
	}
	return toResponse(st), nil
}

// List retrieves all service types (admin default list).
func (s *Service) List(ctx context.Context) (transport.ServiceTypeListResponse, error) {
	items, err := s.repo.List(ctx)
	if err != nil {
		return transport.ServiceTypeListResponse{}, err
	}
	return toListResponseWithPagination(items, len(items), 1, len(items)), nil
}

// ListWithFilters retrieves service types with search, filters, and pagination (admin).
func (s *Service) ListWithFilters(ctx context.Context, req transport.ListServiceTypesRequest) (transport.ServiceTypeListResponse, error) {
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

	isActive := req.IsActive
	if isActive == nil {
		defaultActive := true
		isActive = &defaultActive
	}

	params := repository.ListParams{
		Search:    req.Search,
		IsActive:  isActive,
		Offset:    (page - 1) * pageSize,
		Limit:     pageSize,
		SortBy:    req.SortBy,
		SortOrder: req.SortOrder,
	}

	items, total, err := s.repo.ListWithFilters(ctx, params)
	if err != nil {
		return transport.ServiceTypeListResponse{}, err
	}

	return toListResponseWithPagination(items, total, page, pageSize), nil
}

// ListActive retrieves only active service types.
func (s *Service) ListActive(ctx context.Context) (transport.ServiceTypeListResponse, error) {
	items, err := s.repo.ListActive(ctx)
	if err != nil {
		return transport.ServiceTypeListResponse{}, err
	}
	return toListResponseWithPagination(items, len(items), 1, len(items)), nil
}

// Create creates a new service type.
func (s *Service) Create(ctx context.Context, req transport.CreateServiceTypeRequest) (transport.ServiceTypeResponse, error) {
	displayOrder := 0
	if req.DisplayOrder != nil {
		displayOrder = *req.DisplayOrder
	}

	params := repository.CreateParams{
		Name:         req.Name,
		Slug:         generateSlug(req.Name),
		Description:  req.Description,
		Icon:         req.Icon,
		Color:        req.Color,
		DisplayOrder: displayOrder,
	}

	st, err := s.repo.Create(ctx, params)
	if err != nil {
		return transport.ServiceTypeResponse{}, err
	}

	s.log.Info("service type created", "id", st.ID, "name", st.Name, "slug", st.Slug)
	return toResponse(st), nil
}

// Update updates an existing service type.
func (s *Service) Update(ctx context.Context, id uuid.UUID, req transport.UpdateServiceTypeRequest) (transport.ServiceTypeResponse, error) {
	var slug *string
	if req.Name != nil {
		newSlug := generateSlug(*req.Name)
		slug = &newSlug
	}

	params := repository.UpdateParams{
		ID:           id,
		Name:         req.Name,
		Slug:         slug,
		Description:  req.Description,
		Icon:         req.Icon,
		Color:        req.Color,
		DisplayOrder: req.DisplayOrder,
	}

	st, err := s.repo.Update(ctx, params)
	if err != nil {
		return transport.ServiceTypeResponse{}, err
	}

	s.log.Info("service type updated", "id", st.ID, "name", st.Name)
	return toResponse(st), nil
}

// Delete removes or deactivates a service type based on usage.
func (s *Service) Delete(ctx context.Context, id uuid.UUID) (transport.DeleteServiceTypeResponse, error) {
	used, err := s.repo.HasLeadServices(ctx, id)
	if err != nil {
		return transport.DeleteServiceTypeResponse{}, err
	}

	if used {
		if err := s.repo.SetActive(ctx, id, false); err != nil {
			return transport.DeleteServiceTypeResponse{}, err
		}
		s.log.Info("service type deactivated", "id", id)
		return transport.DeleteServiceTypeResponse{Status: "deactivated"}, nil
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return transport.DeleteServiceTypeResponse{}, err
	}

	s.log.Info("service type deleted", "id", id)
	return transport.DeleteServiceTypeResponse{Status: "deleted"}, nil
}

// ToggleActive toggles the is_active flag for a service type.
func (s *Service) ToggleActive(ctx context.Context, id uuid.UUID) (transport.ServiceTypeResponse, error) {
	// Get current state
	st, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return transport.ServiceTypeResponse{}, err
	}

	// Toggle
	newActive := !st.IsActive
	if err := s.repo.SetActive(ctx, id, newActive); err != nil {
		return transport.ServiceTypeResponse{}, err
	}

	// Get updated record
	st, err = s.repo.GetByID(ctx, id)
	if err != nil {
		return transport.ServiceTypeResponse{}, err
	}

	s.log.Info("service type active toggled", "id", id, "isActive", newActive)
	return toResponse(st), nil
}

// Reorder updates the display order of multiple service types.
func (s *Service) Reorder(ctx context.Context, req transport.ReorderRequest) error {
	items := make([]repository.ReorderItem, len(req.Items))
	for i, item := range req.Items {
		items[i] = repository.ReorderItem{
			ID:           item.ID,
			DisplayOrder: item.DisplayOrder,
		}
	}

	if err := s.repo.Reorder(ctx, items); err != nil {
		return err
	}

	s.log.Info("service types reordered", "count", len(items))
	return nil
}

// Exists checks if a service type exists by ID.
func (s *Service) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	return s.repo.Exists(ctx, id)
}

// toResponse converts a repository ServiceType to transport response.
func toResponse(st repository.ServiceType) transport.ServiceTypeResponse {
	return transport.ServiceTypeResponse{
		ID:           st.ID,
		Name:         st.Name,
		Slug:         st.Slug,
		Description:  st.Description,
		Icon:         st.Icon,
		Color:        st.Color,
		IsActive:     st.IsActive,
		DisplayOrder: st.DisplayOrder,
		CreatedAt:    st.CreatedAt,
		UpdatedAt:    st.UpdatedAt,
	}
}

// toListResponseWithPagination converts a slice of repository ServiceTypes to transport response.
func toListResponseWithPagination(items []repository.ServiceType, total int, page int, pageSize int) transport.ServiceTypeListResponse {
	responses := make([]transport.ServiceTypeResponse, len(items))
	for i, item := range items {
		responses[i] = toResponse(item)
	}
	if pageSize < 1 {
		pageSize = len(items)
	}
	totalPages := 0
	if pageSize > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}
	return transport.ServiceTypeListResponse{
		Items:      responses,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}
}

// generateSlug creates a URL-friendly slug from a name.
func generateSlug(name string) string {
	// Convert to lowercase
	slug := strings.ToLower(name)

	// Replace spaces with hyphens
	slug = strings.ReplaceAll(slug, " ", "-")

	// Remove special characters (keep only alphanumeric and hyphens)
	reg := regexp.MustCompile(`[^a-z0-9-]+`)
	slug = reg.ReplaceAllString(slug, "")

	// Remove multiple consecutive hyphens
	reg = regexp.MustCompile(`-+`)
	slug = reg.ReplaceAllString(slug, "-")

	// Trim hyphens from start and end
	slug = strings.Trim(slug, "-")

	return slug
}
