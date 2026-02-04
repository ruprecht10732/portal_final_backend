package handler

import (
	"net/http"
	"time"

	"portal_final_backend/internal/leads/agent"
	"portal_final_backend/internal/leads/management"
	"portal_final_backend/internal/leads/transport"
	"portal_final_backend/internal/notification/sse"
	"portal_final_backend/platform/httpkit"
	"portal_final_backend/platform/validator"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Handler handles HTTP requests for RAC_leads.
// Uses focused services following vertical slicing pattern.
type Handler struct {
	mgmt         *management.Service
	notesHandler *NotesHandler
	advisor      *agent.LeadAdvisor
	callLogger   *agent.CallLogger
	sse          *sse.Service
	val          *validator.Validator
}

const (
	msgInvalidRequest   = "invalid request"
	msgValidationFailed = "validation failed"
	msgTenantRequired   = "tenant context required"
	msgInvalidServiceID = "invalid serviceId"
	dateLayout          = "2006-01-02"
)

// mustGetTenantID extracts and dereferences the tenant ID from identity.
// Returns the tenant ID and true if valid, or handles the error response and returns false.
func mustGetTenantID(c *gin.Context, identity httpkit.Identity) (uuid.UUID, bool) {
	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusForbidden, msgTenantRequired, nil)
		return uuid.UUID{}, false
	}
	return *tenantID, true
}

// New creates a new RAC_leads handler with focused services.
func New(mgmt *management.Service, notesHandler *NotesHandler, advisor *agent.LeadAdvisor, callLogger *agent.CallLogger, sseService *sse.Service, val *validator.Validator) *Handler {
	return &Handler{mgmt: mgmt, notesHandler: notesHandler, advisor: advisor, callLogger: callLogger, sse: sseService, val: val}
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("", h.List)
	rg.POST("", h.Create)
	rg.GET("/metrics", h.GetMetrics)
	rg.GET("/heatmap", h.GetHeatmap)
	rg.GET("/action-items", h.GetActionItems)
	rg.GET("/check-duplicate", h.CheckDuplicate)
	rg.GET("/check-returning-customer", h.CheckReturningCustomer)
	rg.GET("/:id", h.GetByID)
	rg.PUT("/:id", h.Update)
	rg.DELETE("/:id", h.Delete)
	rg.POST("/bulk-delete", h.BulkDelete)
	rg.PATCH("/:id/status", h.UpdateStatus)
	rg.PUT(":id/assign", h.Assign)
	rg.POST("/:id/view", h.MarkViewed)
	rg.GET("/:id/notes", h.notesHandler.ListNotes)
	rg.POST("/:id/notes", h.notesHandler.AddNote)
	// Service-specific routes
	rg.POST("/:id/services", h.AddService)
	rg.PATCH("/:id/services/:serviceId/status", h.UpdateServiceStatus)
	// AI Advisor routes
	rg.POST("/:id/analyze", h.AnalyzeLead)
	rg.GET("/:id/analysis", h.GetAnalysis)
	rg.GET("/:id/analysis/history", h.ListAnalyses)
	// Call Logger routes
	rg.POST("/:id/services/:serviceId/log-call", h.LogCall)
}

func (h *Handler) GetMetrics(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	metrics, err := h.mgmt.GetMetrics(c.Request.Context(), tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, metrics)
}

func (h *Handler) GetHeatmap(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	var req transport.LeadHeatmapRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	startDate, endDate, errMsg := parseDateRange(req.StartDate, req.EndDate)
	if errMsg != "" {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, errMsg)
		return
	}

	result, err := h.mgmt.GetHeatmap(c.Request.Context(), startDate, endDate, tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

func (h *Handler) GetActionItems(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	var req transport.ActionItemsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 {
		req.PageSize = 5
	}
	if req.PageSize > 50 {
		req.PageSize = 50
	}

	result, err := h.mgmt.GetActionItems(c.Request.Context(), req.Page, req.PageSize, 7, tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

func (h *Handler) Create(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	var req transport.CreateLeadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	lead, err := h.mgmt.Create(c.Request.Context(), req, tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	h.publishLeadUpdate(tenantID, &lead.ID, "created")
	httpkit.JSON(c, http.StatusCreated, lead)
}

func (h *Handler) GetByID(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	lead, err := h.mgmt.GetByID(c.Request.Context(), id, tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, lead)
}

func (h *Handler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.UpdateLeadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	lead, err := h.mgmt.Update(c.Request.Context(), id, req, identity.UserID(), tenantID, identity.Roles())
	if httpkit.HandleError(c, err) {
		return
	}

	h.publishLeadUpdate(tenantID, &lead.ID, "updated")
	httpkit.OK(c, lead)
}

func (h *Handler) Assign(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.AssignLeadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	lead, err := h.mgmt.Assign(c.Request.Context(), id, req.AssigneeID, identity.UserID(), tenantID, identity.Roles())
	if httpkit.HandleError(c, err) {
		return
	}

	h.publishLeadUpdate(tenantID, &lead.ID, "assigned")
	httpkit.OK(c, lead)
}

func (h *Handler) Delete(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	if err := h.mgmt.Delete(c.Request.Context(), id, tenantID); httpkit.HandleError(c, err) {
		return
	}

	h.publishLeadUpdate(tenantID, &id, "deleted")
	httpkit.OK(c, gin.H{"message": "lead deleted"})
}

func (h *Handler) BulkDelete(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	var req transport.BulkDeleteLeadsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	deletedCount, err := h.mgmt.BulkDelete(c.Request.Context(), req.IDs, tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	h.publishLeadUpdate(tenantID, nil, "bulk_deleted")
	httpkit.OK(c, transport.BulkDeleteLeadsResponse{DeletedCount: deletedCount})
}

func (h *Handler) UpdateStatus(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.UpdateLeadStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	lead, err := h.mgmt.UpdateStatus(c.Request.Context(), id, req, tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	h.publishLeadUpdate(tenantID, &lead.ID, "status_updated")
	httpkit.OK(c, lead)
}

func (h *Handler) MarkViewed(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	if err := h.mgmt.SetViewedBy(c.Request.Context(), id, identity.UserID(), tenantID); httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, gin.H{"message": "lead marked as viewed"})
}

func (h *Handler) CheckDuplicate(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	phone := c.Query("phone")
	if phone == "" {
		httpkit.Error(c, http.StatusBadRequest, "phone parameter required", nil)
		return
	}

	result, err := h.mgmt.CheckDuplicate(c.Request.Context(), phone, tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

func (h *Handler) CheckReturningCustomer(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	phone := c.Query("phone")
	email := c.Query("email")

	if phone == "" && email == "" {
		httpkit.Error(c, http.StatusBadRequest, "phone or email parameter required", nil)
		return
	}

	result, err := h.mgmt.CheckReturningCustomer(c.Request.Context(), phone, email, tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

func (h *Handler) List(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	var req transport.ListLeadsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	result, err := h.mgmt.List(c.Request.Context(), req, tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

func (h *Handler) AddService(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.AddServiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	lead, err := h.mgmt.AddService(c.Request.Context(), id, req, tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.JSON(c, http.StatusCreated, lead)
}

func (h *Handler) publishLeadUpdate(tenantID uuid.UUID, leadID *uuid.UUID, action string) {
	if h.sse == nil {
		return
	}

	event := sse.Event{
		Type:    sse.EventLeadUpdated,
		Message: "Lead updated",
		Data:    gin.H{"action": action},
	}
	if leadID != nil {
		event.LeadID = *leadID
	}

	h.sse.PublishToOrganization(tenantID, event)
}

func (h *Handler) UpdateServiceStatus(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	leadID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	serviceID, err := uuid.Parse(c.Param("serviceId"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.UpdateServiceStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	lead, err := h.mgmt.UpdateServiceStatus(c.Request.Context(), leadID, serviceID, req, tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, lead)
}

// AnalyzeLead triggers AI analysis for a lead service and returns the result
func (h *Handler) AnalyzeLead(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	// Validate optional serviceId with terminal status check
	validation := h.validateServiceForAnalysis(c, c.Query("serviceId"), tenantID)
	if validation.ErrMsg != "" {
		httpkit.Error(c, validation.ErrStatus, validation.ErrMsg, nil)
		return
	}

	// Check for force parameter to bypass no_change detection
	force := c.Query("force") == "true"

	response, err := h.advisor.AnalyzeAndReturn(c.Request.Context(), id, validation.ServiceID, force, tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, response)
}

// GetAnalysis returns the latest AI analysis for a lead service
func (h *Handler) GetAnalysis(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	// Parse required serviceId
	svcID := c.Query("serviceId")
	if svcID == "" {
		httpkit.Error(c, http.StatusBadRequest, "serviceId parameter required", nil)
		return
	}
	serviceID, err := uuid.Parse(svcID)
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidServiceID, nil)
		return
	}

	analysis, hasAnalysis, err := h.advisor.GetLatestOrDefault(c.Request.Context(), id, serviceID, tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, gin.H{
		"analysis":  analysis,
		"isDefault": !hasAnalysis,
	})
}

// ListAnalyses returns all AI analyses for a lead service
func (h *Handler) ListAnalyses(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	_, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	// Parse required serviceId
	svcID := c.Query("serviceId")
	if svcID == "" {
		httpkit.Error(c, http.StatusBadRequest, "serviceId parameter required", nil)
		return
	}
	serviceID, err := uuid.Parse(svcID)
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidServiceID, nil)
		return
	}

	analyses, err := h.advisor.ListAnalyses(c.Request.Context(), serviceID, tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, gin.H{"items": analyses})
}

// LogCall processes a post-call summary and executes appropriate actions (notes, status updates, appointments)
func (h *Handler) LogCall(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	leadID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	serviceID, err := uuid.Parse(c.Param("serviceId"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.LogCallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return
	}

	result, err := h.callLogger.ProcessSummary(c.Request.Context(), leadID, serviceID, identity.UserID(), tenantID, req.Summary)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

// isTerminalStatus checks if a service status is terminal (no further actions allowed)
func isTerminalStatus(status string) bool {
	switch status {
	case "Closed", "Bad_Lead", "Surveyed":
		return true
	default:
		return false
	}
}

// parseDateRange parses optional start and end date strings and validates the range.
// Returns nil dates for empty strings. Returns an error message if parsing fails or dates are invalid.
func parseDateRange(startDateStr, endDateStr string) (startDate, endDate *time.Time, errMsg string) {
	if startDateStr != "" {
		parsed, err := time.Parse(dateLayout, startDateStr)
		if err != nil {
			return nil, nil, "invalid startDate"
		}
		startDate = &parsed
	}

	if endDateStr != "" {
		parsed, err := time.Parse(dateLayout, endDateStr)
		if err != nil {
			return nil, nil, "invalid endDate"
		}
		endDate = &parsed
	}

	if startDate != nil && endDate != nil && startDate.After(*endDate) {
		return nil, nil, "startDate must be before or equal to endDate"
	}

	return startDate, endDate, ""
}

// serviceValidationResult holds the result of validating a service ID for analysis.
type serviceValidationResult struct {
	ServiceID *uuid.UUID
	ErrMsg    string
	ErrStatus int
}

// validateServiceForAnalysis validates and parses an optional service ID, checking terminal status.
func (h *Handler) validateServiceForAnalysis(ctx *gin.Context, svcIDStr string, tenantID uuid.UUID) serviceValidationResult {
	if svcIDStr == "" {
		return serviceValidationResult{}
	}

	parsed, err := uuid.Parse(svcIDStr)
	if err != nil {
		return serviceValidationResult{ErrMsg: msgInvalidServiceID, ErrStatus: http.StatusBadRequest}
	}

	service, err := h.mgmt.GetLeadServiceByID(ctx.Request.Context(), parsed, tenantID)
	if err != nil {
		return serviceValidationResult{ErrMsg: "service not found", ErrStatus: http.StatusNotFound}
	}

	if isTerminalStatus(service.Status) {
		return serviceValidationResult{ErrMsg: "cannot analyze a service in terminal status (Closed, Bad_Lead, Surveyed)", ErrStatus: http.StatusBadRequest}
	}

	return serviceValidationResult{ServiceID: &parsed}
}
