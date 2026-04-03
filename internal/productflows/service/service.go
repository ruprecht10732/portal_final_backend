package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"portal_final_backend/internal/productflows/repository"
	"portal_final_backend/internal/productflows/transport"
	"portal_final_backend/platform/logger"
)

// Service provides business logic for product flows.
type Service struct {
	repo repository.Repository
	log  *logger.Logger
}

// New creates a new product-flows service.
func New(repo repository.Repository, log *logger.Logger) *Service {
	return &Service{repo: repo, log: log}
}

// GetFlow returns the active flow definition for a product group,
// with tenant override → global fallback.
func (s *Service) GetFlow(ctx context.Context, tenantID uuid.UUID, productGroupID string) (transport.ProductFlowResponse, error) {
	flow, err := s.repo.GetActiveFlow(ctx, tenantID, productGroupID)
	if err != nil {
		return transport.ProductFlowResponse{}, err
	}
	return toResponse(flow), nil
}

// ListAll returns all active flows visible to the tenant.
func (s *Service) ListAll(ctx context.Context, tenantID uuid.UUID) (transport.ProductFlowListResponse, error) {
	flows, err := s.repo.ListAll(ctx, tenantID)
	if err != nil {
		return transport.ProductFlowListResponse{}, err
	}
	items := make([]transport.ProductFlowResponse, 0, len(flows))
	for _, f := range flows {
		items = append(items, toResponse(f))
	}
	return transport.ProductFlowListResponse{Items: items}, nil
}

// Create inserts a new flow definition under the given org.
func (s *Service) Create(ctx context.Context, tenantID uuid.UUID, req transport.CreateProductFlowRequest) (transport.ProductFlowResponse, error) {
	defJSON, err := json.Marshal(req.Definition)
	if err != nil {
		return transport.ProductFlowResponse{}, fmt.Errorf("marshal definition: %w", err)
	}
	var edJSON json.RawMessage
	if req.EditorDefinition != nil {
		edJSON, err = json.Marshal(req.EditorDefinition)
		if err != nil {
			return transport.ProductFlowResponse{}, fmt.Errorf("marshal editor definition: %w", err)
		}
	}
	flow, err := s.repo.Create(ctx, tenantID, req.ProductGroupID, defJSON, edJSON)
	if err != nil {
		return transport.ProductFlowResponse{}, err
	}
	return toResponse(flow), nil
}

// Update replaces the definition for an existing flow.
func (s *Service) Update(ctx context.Context, tenantID uuid.UUID, flowID uuid.UUID, req transport.UpdateProductFlowRequest) (transport.ProductFlowResponse, error) {
	defJSON, err := json.Marshal(req.Definition)
	if err != nil {
		return transport.ProductFlowResponse{}, fmt.Errorf("marshal definition: %w", err)
	}
	var edJSON json.RawMessage
	if req.EditorDefinition != nil {
		edJSON, err = json.Marshal(req.EditorDefinition)
		if err != nil {
			return transport.ProductFlowResponse{}, fmt.Errorf("marshal editor definition: %w", err)
		}
	}
	flow, err := s.repo.Update(ctx, flowID, tenantID, defJSON, edJSON)
	if err != nil {
		return transport.ProductFlowResponse{}, err
	}
	return toResponse(flow), nil
}

// Delete soft-deletes a flow owned by the tenant.
func (s *Service) Delete(ctx context.Context, tenantID uuid.UUID, flowID uuid.UUID) error {
	return s.repo.Delete(ctx, flowID, tenantID)
}

// Duplicate copies an existing flow into a new row for the tenant.
func (s *Service) Duplicate(ctx context.Context, tenantID uuid.UUID, flowID uuid.UUID) (transport.ProductFlowResponse, error) {
	flow, err := s.repo.Duplicate(ctx, flowID, tenantID)
	if err != nil {
		return transport.ProductFlowResponse{}, err
	}
	return toResponse(flow), nil
}

func toResponse(f repository.ProductFlow) transport.ProductFlowResponse {
	var def any
	_ = json.Unmarshal(f.Definition, &def)

	var edDef any
	if len(f.EditorDefinition) > 0 {
		_ = json.Unmarshal(f.EditorDefinition, &edDef)
	}

	return transport.ProductFlowResponse{
		ID:               f.ID,
		ProductGroupID:   f.ProductGroupID,
		Version:          f.Version,
		IsGlobal:         f.OrganizationID == nil,
		Definition:       def,
		EditorDefinition: edDef,
	}
}
