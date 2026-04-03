package transport

import "github.com/google/uuid"

// ProductFlowResponse wraps the FlowDefinition JSON returned to the client.
type ProductFlowResponse struct {
	ID               uuid.UUID `json:"id"`
	ProductGroupID   string    `json:"productGroupId"`
	Version          int       `json:"version"`
	IsGlobal         bool      `json:"isGlobal"`
	Definition       any       `json:"definition"`
	EditorDefinition any       `json:"editorDefinition,omitempty"`
}

// CreateProductFlowRequest is the admin request to create a new flow.
type CreateProductFlowRequest struct {
	ProductGroupID   string `json:"productGroupId" validate:"required,min=1,max=100"`
	Definition       any    `json:"definition" validate:"required"`
	EditorDefinition any    `json:"editorDefinition,omitempty"`
}

// UpdateProductFlowRequest is the admin request to update a flow definition.
type UpdateProductFlowRequest struct {
	Definition       any `json:"definition" validate:"required"`
	EditorDefinition any `json:"editorDefinition,omitempty"`
}

// ProductFlowListResponse wraps a list of product flows for admin listing.
type ProductFlowListResponse struct {
	Items []ProductFlowResponse `json:"items"`
}
