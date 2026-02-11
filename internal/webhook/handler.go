package webhook

import (
	"net/http"
	"strconv"
	"time"

	"portal_final_backend/platform/httpkit"
	"portal_final_backend/platform/validator"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	errNoOrgContext      = "no organization context"
	errInvalidRequest    = "invalid request body"
	errValidation        = "validation error"
	errInvalidConfigID   = "invalid config ID"
	googleTimeFormat     = "2006-01-02T15:04:05Z"
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
	orgID, ok := h.getWebhookOrgID(c)
	if !ok {
		return
	}
	apiKeyID, _ := c.Get("webhookKeyID")

	submission, ok := h.parseFormSubmission(c, apiKeyID)
	if !ok {
		return
	}

	resp, err := h.service.ProcessFormSubmission(c.Request.Context(), submission, orgID)
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
	var req CreateAPIKeyRequest
	if !h.bindAndValidate(c, &req) {
		return
	}

	tenantID, ok := h.getTenantID(c)
	if !ok {
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

	key, err := h.repo.Create(c.Request.Context(), tenantID, req.Name, hash, prefix, domains)
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
	tenantID, ok := h.getTenantID(c)
	if !ok {
		return
	}

	keys, err := h.repo.ListByOrganization(c.Request.Context(), tenantID)
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
	tenantID, ok := h.getTenantID(c)
	if !ok {
		return
	}

	keyID, err := uuid.Parse(c.Param("keyId"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, "invalid key ID", nil)
		return
	}

	if err := h.repo.Revoke(c.Request.Context(), keyID, tenantID); err != nil {
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
		CreatedAt:      key.CreatedAt.Format(googleTimeFormat),
	}
}

// ---- Google Lead Forms Webhooks ----

// GoogleLeadWebhookResponse is returned for webhook processing results.
type GoogleLeadWebhookResponse struct {
	LeadID      *uuid.UUID `json:"leadId,omitempty"`
	IsTest      bool       `json:"isTest"`
	IsDuplicate bool       `json:"isDuplicate"`
	Message     string     `json:"message"`
}

// HandleGoogleLeadWebhook processes Google Lead Form webhook payloads.
// POST /api/v1/webhook/google-leads
func (h *Handler) HandleGoogleLeadWebhook(c *gin.Context) {
	var payload GoogleLeadPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpkit.Error(c, http.StatusBadRequest, "invalid payload", err.Error())
		return
	}
	if payload.GoogleKey == "" {
		httpkit.Error(c, http.StatusUnauthorized, "missing google_key", nil)
		return
	}

	keyHash := HashKey(payload.GoogleKey)
	config, err := h.repo.GetGoogleConfigByKey(c.Request.Context(), keyHash)
	if err != nil {
		if err == ErrGoogleConfigNotFound {
			httpkit.Error(c, http.StatusUnauthorized, "invalid google_key", nil)
			return
		}
		httpkit.HandleError(c, err)
		return
	}

	result, err := h.service.ProcessGoogleLeadWebhook(c.Request.Context(), payload, config)
	if httpkit.HandleError(c, err) {
		return
	}

	message := "Lead created"
	if result.IsTest {
		message = "Test lead received"
	}
	if result.IsDuplicate {
		message = "Duplicate lead ignored"
	}

	c.JSON(http.StatusOK, GoogleLeadWebhookResponse{
		LeadID:      result.LeadID,
		IsTest:      result.IsTest,
		IsDuplicate: result.IsDuplicate,
		Message:     message,
	})
}

// HandleCreateGoogleWebhookConfig creates a new Google webhook config.
// POST /api/v1/admin/webhook/google-lead-config
func (h *Handler) HandleCreateGoogleWebhookConfig(c *gin.Context) {
	var req CreateGoogleWebhookConfigRequest
	if !h.bindAndValidate(c, &req) {
		return
	}

	tenantID, ok := h.getTenantID(c)
	if !ok {
		return
	}

	plaintext, hash, prefix, err := GenerateGoogleKey()
	if err != nil {
		httpkit.Error(c, http.StatusInternalServerError, "failed to generate google_key", nil)
		return
	}

	configID, err := h.repo.CreateGoogleConfig(c.Request.Context(), tenantID, req.Name, hash, prefix)
	if httpkit.HandleError(c, err) {
		return
	}

	webhookURL := buildWebhookURL(c, "/api/v1/webhook/google-leads")

	c.JSON(http.StatusCreated, CreateGoogleWebhookConfigResponse{
		ID:              configID,
		Name:            req.Name,
		GoogleKey:       plaintext,
		GoogleKeyPrefix: prefix,
		WebhookURL:      webhookURL,
		CreatedAt:       time.Now().UTC().Format(googleTimeFormat),
	})
}

// HandleListGoogleWebhookConfigs lists Google webhook configs for the tenant.
// GET /api/v1/admin/webhook/google-lead-config
func (h *Handler) HandleListGoogleWebhookConfigs(c *gin.Context) {
	tenantID, ok := h.getTenantID(c)
	if !ok {
		return
	}

	configs, err := h.repo.ListGoogleConfigs(c.Request.Context(), tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	result := make([]GoogleWebhookConfigResponse, len(configs))
	for i, cfg := range configs {
		result[i] = GoogleWebhookConfigResponse{
			ID:               cfg.ID,
			Name:             cfg.Name,
			GoogleKeyPrefix:  cfg.GoogleKeyPrefix,
			CampaignMappings: cfg.CampaignMappings,
			IsActive:         cfg.IsActive,
			CreatedAt:        cfg.CreatedAt.Format(googleTimeFormat),
			UpdatedAt:        cfg.UpdatedAt.Format(googleTimeFormat),
		}
	}

	httpkit.OK(c, result)
}

// HandleUpdateGoogleCampaignMapping updates campaign mapping.
// PUT /api/v1/admin/webhook/google-lead-config/:configId/campaigns
func (h *Handler) HandleUpdateGoogleCampaignMapping(c *gin.Context) {
	tenantID, ok := h.getTenantID(c)
	if !ok {
		return
	}

	configID, ok := h.parseConfigID(c)
	if !ok {
		return
	}

	var req UpdateCampaignMappingRequest
	if !h.bindAndValidate(c, &req) {
		return
	}

	campaignID, err := strconv.ParseInt(req.CampaignID, 10, 64)
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, "invalid campaign ID", nil)
		return
	}

	if err := h.repo.UpdateGoogleCampaignMapping(c.Request.Context(), configID, tenantID, campaignID, req.ServiceType); err != nil {
		httpkit.HandleError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// HandleDeleteGoogleCampaignMapping deletes a campaign mapping.
// DELETE /api/v1/admin/webhook/google-lead-config/:configId/campaigns/:campaignId
func (h *Handler) HandleDeleteGoogleCampaignMapping(c *gin.Context) {
	tenantID, ok := h.getTenantID(c)
	if !ok {
		return
	}

	configID, ok := h.parseConfigID(c)
	if !ok {
		return
	}

	campaignID, err := strconv.ParseInt(c.Param("campaignId"), 10, 64)
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, "invalid campaign ID", nil)
		return
	}

	if err := h.repo.DeleteGoogleCampaignMapping(c.Request.Context(), configID, tenantID, campaignID); err != nil {
		httpkit.HandleError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// HandleDeleteGoogleWebhookConfig deletes a Google webhook configuration.
// DELETE /api/v1/admin/webhook/google-lead-config/:configId
func (h *Handler) HandleDeleteGoogleWebhookConfig(c *gin.Context) {
	tenantID, ok := h.getTenantID(c)
	if !ok {
		return
	}

	configID, ok := h.parseConfigID(c)
	if !ok {
		return
	}

	if err := h.repo.DeleteGoogleConfig(c.Request.Context(), configID, tenantID); err != nil {
		if err == ErrGoogleConfigNotFound {
			httpkit.Error(c, http.StatusNotFound, "config not found", nil)
			return
		}
		httpkit.HandleError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

func buildWebhookURL(c *gin.Context, path string) string {
	scheme := "https"
	if c.Request.TLS == nil {
		scheme = "http"
	}
	if forwarded := c.GetHeader("X-Forwarded-Proto"); forwarded != "" {
		scheme = forwarded
	}
	return scheme + "://" + c.Request.Host + path
}

func (h *Handler) getTenantID(c *gin.Context) (uuid.UUID, bool) {
	identity := httpkit.MustGetIdentity(c)
	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusForbidden, errNoOrgContext, nil)
		return uuid.UUID{}, false
	}
	return *tenantID, true
}

func (h *Handler) parseConfigID(c *gin.Context) (uuid.UUID, bool) {
	configID, err := uuid.Parse(c.Param("configId"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, errInvalidConfigID, nil)
		return uuid.UUID{}, false
	}
	return configID, true
}

func (h *Handler) bindAndValidate(c *gin.Context, req interface{}) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, errInvalidRequest, err.Error())
		return false
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, errValidation, err.Error())
		return false
	}
	return true
}

func (h *Handler) getWebhookOrgID(c *gin.Context) (uuid.UUID, bool) {
	orgID, ok := c.Get("webhookOrgID")
	if !ok {
		httpkit.Error(c, http.StatusUnauthorized, "missing organization context", nil)
		return uuid.UUID{}, false
	}
	return orgID.(uuid.UUID), true
}

func (h *Handler) parseFormSubmission(c *gin.Context, apiKeyID interface{}) (FormSubmission, bool) {
	if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
		if err := c.Request.ParseForm(); err != nil {
			httpkit.Error(c, http.StatusBadRequest, "unable to parse form data", nil)
			return FormSubmission{}, false
		}
	}

	fields := h.collectFormFields(c)
	files := h.collectFormFiles(c)
	h.collectJSONFields(c, fields)

	if len(fields) == 0 && len(files) == 0 {
		httpkit.Error(c, http.StatusBadRequest, "no form data received", nil)
		return FormSubmission{}, false
	}

	submission := FormSubmission{
		Fields:       fields,
		Files:        files,
		SourceDomain: c.GetHeader("Origin"),
	}
	if keyID, ok := apiKeyID.(uuid.UUID); ok {
		submission.APIKeyID = keyID
	}

	return submission, true
}

func (h *Handler) collectFormFields(c *gin.Context) map[string]string {
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
	return fields
}

func (h *Handler) collectFormFiles(c *gin.Context) []FormFile {
	var files []FormFile
	if c.Request.MultipartForm == nil {
		return files
	}
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
	return files
}

func (h *Handler) collectJSONFields(c *gin.Context, fields map[string]string) {
	if c.ContentType() != "application/json" {
		return
	}

	var jsonBody map[string]interface{}
	if err := c.ShouldBindJSON(&jsonBody); err != nil {
		return
	}
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
