package webhook

import (
	"net/http"

	"portal_final_backend/platform/httpkit"
	"portal_final_backend/platform/validator"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Handler handles webhook HTTP requests.
type Handler struct {
	service *Service
	repo    *Repository
	val     *validator.Validator
}

// NewHandler creates a new webhook handler.
func NewHandler(service *Service, repo *Repository, val *validator.Validator) *Handler {
	return &Handler{service: service, repo: repo, val: val}
}

// ---- Form Submission (public, API-key authenticated) ----

// HandleFormSubmission processes an inbound form submission.
// POST /api/v1/webhook/forms
// Authenticated via X-Webhook-API-Key header (set by middleware).
func (h *Handler) HandleFormSubmission(c *gin.Context) {
	// orgID and apiKeyID are set by the API key auth middleware
	orgID, ok := c.Get("webhookOrgID")
	if !ok {
		httpkit.Error(c, http.StatusUnauthorized, "missing organization context", nil)
		return
	}
	apiKeyID, _ := c.Get("webhookKeyID")

	// Parse multipart form (max 32 MB)
	if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
		// Fallback: try regular form data
		if err := c.Request.ParseForm(); err != nil {
			httpkit.Error(c, http.StatusBadRequest, "unable to parse form data", nil)
			return
		}
	}

	// Collect all text fields
	fields := make(map[string]string)
	if c.Request.MultipartForm != nil {
		for key, values := range c.Request.MultipartForm.Value {
			if len(values) > 0 {
				fields[key] = values[0]
			}
		}
	}
	for key, values := range c.Request.PostForm {
		if _, exists := fields[key]; !exists && len(values) > 0 {
			fields[key] = values[0]
		}
	}

	// Collect files
	var files []FormFile
	if c.Request.MultipartForm != nil {
		for fieldName, fileHeaders := range c.Request.MultipartForm.File {
			for _, fh := range fileHeaders {
				f, err := fh.Open()
				if err != nil {
					continue
				}
				files = append(files, FormFile{
					FieldName:   fieldName,
					FileName:    fh.Filename,
					ContentType: fh.Header.Get("Content-Type"),
					Size:        fh.Size,
					Reader:      f,
				})
			}
		}
	}

	// Also support JSON body (for SDK sending JSON)
	if c.ContentType() == "application/json" {
		var jsonBody map[string]interface{}
		if err := c.ShouldBindJSON(&jsonBody); err == nil {
			for key, val := range jsonBody {
				switch v := val.(type) {
				case string:
					fields[key] = v
				case float64:
					fields[key] = c.DefaultQuery(key, "")
					_ = v
				}
			}
		}
	}

	if len(fields) == 0 && len(files) == 0 {
		httpkit.Error(c, http.StatusBadRequest, "no form data received", nil)
		return
	}

	submission := FormSubmission{
		Fields:       fields,
		Files:        files,
		SourceDomain: c.GetHeader("Origin"),
	}
	if keyID, ok := apiKeyID.(uuid.UUID); ok {
		submission.APIKeyID = keyID
	}

	resp, err := h.service.ProcessFormSubmission(c.Request.Context(), submission, orgID.(uuid.UUID))
	if httpkit.HandleError(c, err) {
		return
	}

	c.JSON(http.StatusCreated, resp)
}

// ---- Admin API Key Management (JWT authenticated) ----

// CreateAPIKeyRequest is the request body for creating a new API key.
type CreateAPIKeyRequest struct {
	Name           string   `json:"name" validate:"required,min=1,max=100"`
	AllowedDomains []string `json:"allowedDomains" validate:"max=20,dive,max=200"`
}

// APIKeyResponse is returned when listing or creating API keys.
type APIKeyResponse struct {
	ID             uuid.UUID `json:"id"`
	Name           string    `json:"name"`
	KeyPrefix      string    `json:"keyPrefix"`
	AllowedDomains []string  `json:"allowedDomains"`
	IsActive       bool      `json:"isActive"`
	CreatedAt      string    `json:"createdAt"`
}

// CreateAPIKeyResponse includes the plaintext key (shown only once).
type CreateAPIKeyResponse struct {
	APIKeyResponse
	Key string `json:"key"` // plaintext, shown only once
}

// HandleCreateAPIKey creates a new webhook API key.
// POST /api/v1/admin/webhook/keys
func (h *Handler) HandleCreateAPIKey(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusForbidden, "no organization context", nil)
		return
	}

	var req CreateAPIKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, "validation error", err.Error())
		return
	}

	plaintext, hash, prefix, err := GenerateAPIKey()
	if err != nil {
		httpkit.Error(c, http.StatusInternalServerError, "failed to generate API key", nil)
		return
	}

	domains := req.AllowedDomains
	if domains == nil {
		domains = []string{}
	}

	key, err := h.repo.Create(c.Request.Context(), *tenantID, req.Name, hash, prefix, domains)
	if httpkit.HandleError(c, err) {
		return
	}

	c.JSON(http.StatusCreated, CreateAPIKeyResponse{
		APIKeyResponse: toAPIKeyResponse(key),
		Key:            plaintext,
	})
}

// HandleListAPIKeys lists all webhook API keys for the organization.
// GET /api/v1/admin/webhook/keys
func (h *Handler) HandleListAPIKeys(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusForbidden, "no organization context", nil)
		return
	}

	keys, err := h.repo.ListByOrganization(c.Request.Context(), *tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	result := make([]APIKeyResponse, len(keys))
	for i, k := range keys {
		result[i] = toAPIKeyResponse(k)
	}

	httpkit.OK(c, result)
}

// HandleRevokeAPIKey deactivates a webhook API key.
// DELETE /api/v1/admin/webhook/keys/:keyId
func (h *Handler) HandleRevokeAPIKey(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusForbidden, "no organization context", nil)
		return
	}

	keyID, err := uuid.Parse(c.Param("keyId"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, "invalid key ID", nil)
		return
	}

	if err := h.repo.Revoke(c.Request.Context(), keyID, *tenantID); err != nil {
		if err == ErrAPIKeyNotFound {
			httpkit.Error(c, http.StatusNotFound, "API key not found", nil)
			return
		}
		httpkit.HandleError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "API key revoked"})
}

func toAPIKeyResponse(key APIKey) APIKeyResponse {
	return APIKeyResponse{
		ID:             key.ID,
		Name:           key.Name,
		KeyPrefix:      key.KeyPrefix,
		AllowedDomains: key.AllowedDomains,
		IsActive:       key.IsActive,
		CreatedAt:      key.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}
