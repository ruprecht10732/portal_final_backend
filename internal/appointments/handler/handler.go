package handler

import (
	"net/http"

	"portal_final_backend/internal/appointments/service"
	"portal_final_backend/internal/appointments/transport"
	"portal_final_backend/platform/httpkit"
	"portal_final_backend/platform/validator"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	msgInvalidRequest   = "invalid request"
	msgValidationFailed = "validation failed"
)

// Handler handles HTTP requests for appointments
type Handler struct {
	svc *service.Service
	val *validator.Validator
}

// New creates a new appointments handler
func New(svc *service.Service, val *validator.Validator) *Handler {
	return &Handler{svc: svc, val: val}
}

// mustGetTenantID extracts the tenant ID from identity and returns it.
// Returns zero UUID and false if tenant ID is not present.
func mustGetTenantID(c *gin.Context, identity httpkit.Identity) (uuid.UUID, bool) {
	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusBadRequest, "tenant ID is required", nil)
		return uuid.UUID{}, false
	}
	return *tenantID, true
}

// RegisterRoutes registers the appointment routes
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("", h.List)
	rg.POST("", h.Create)
	rg.GET("/:id", h.GetByID)
	rg.PUT("/:id", h.Update)
	rg.DELETE("/:id", h.Delete)
	rg.PATCH("/:id/status", h.UpdateStatus)
	rg.GET("/:id/visit-report", h.GetVisitReport)
	rg.PUT("/:id/visit-report", h.UpsertVisitReport)
	rg.GET("/:id/attachments", h.ListAttachments)
	rg.POST("/:id/attachments", h.CreateAttachment)

	rg.GET("/availability/rules", h.ListAvailabilityRules)
	rg.POST("/availability/rules", h.CreateAvailabilityRule)
	rg.DELETE("/availability/rules/:id", h.DeleteAvailabilityRule)

	rg.GET("/availability/overrides", h.ListAvailabilityOverrides)
	rg.POST("/availability/overrides", h.CreateAvailabilityOverride)
	rg.DELETE("/availability/overrides/:id", h.DeleteAvailabilityOverride)

	rg.GET("/availability/slots", h.GetAvailableSlots)
}

// List handles GET /api/appointments
func (h *Handler) List(c *gin.Context) {
	var req transport.ListAppointmentsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, err.Error())
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

	isAdmin := containsRole(identity.Roles(), "admin")
	result, err := h.svc.List(c.Request.Context(), identity.UserID(), isAdmin, tenantID, req)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

// Create handles POST /api/appointments
func (h *Handler) Create(c *gin.Context) {
	var req transport.CreateAppointmentRequest
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

	isAdmin := containsRole(identity.Roles(), "admin")
	result, err := h.svc.Create(c.Request.Context(), identity.UserID(), isAdmin, tenantID, req)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.JSON(c, http.StatusCreated, result)
}

// GetByID handles GET /api/appointments/:id
func (h *Handler) GetByID(c *gin.Context) {
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

	isAdmin := containsRole(identity.Roles(), "admin")
	result, err := h.svc.GetByID(c.Request.Context(), id, identity.UserID(), isAdmin, tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

// Update handles PUT /api/appointments/:id
func (h *Handler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.UpdateAppointmentRequest
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

	isAdmin := containsRole(identity.Roles(), "admin")
	result, err := h.svc.Update(c.Request.Context(), id, identity.UserID(), isAdmin, tenantID, req)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

// Delete handles DELETE /api/appointments/:id
func (h *Handler) Delete(c *gin.Context) {
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

	isAdmin := containsRole(identity.Roles(), "admin")
	if err := h.svc.Delete(c.Request.Context(), id, identity.UserID(), isAdmin, tenantID); httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, gin.H{"message": "appointment deleted"})
}

// UpdateStatus handles PATCH /api/appointments/:id/status
func (h *Handler) UpdateStatus(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.UpdateAppointmentStatusRequest
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

	isAdmin := containsRole(identity.Roles(), "admin")
	result, err := h.svc.UpdateStatus(c.Request.Context(), id, identity.UserID(), isAdmin, tenantID, req)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

// GetVisitReport handles GET /api/appointments/:id/visit-report
func (h *Handler) GetVisitReport(c *gin.Context) {
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

	isAdmin := containsRole(identity.Roles(), "admin")
	result, err := h.svc.GetVisitReport(c.Request.Context(), id, identity.UserID(), isAdmin, tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

// UpsertVisitReport handles PUT /api/appointments/:id/visit-report
func (h *Handler) UpsertVisitReport(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.UpsertVisitReportRequest
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

	isAdmin := containsRole(identity.Roles(), "admin")
	result, err := h.svc.UpsertVisitReport(c.Request.Context(), id, identity.UserID(), isAdmin, tenantID, req)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

// CreateAttachment handles POST /api/appointments/:id/attachments
func (h *Handler) CreateAttachment(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return
	}

	var req transport.CreateAppointmentAttachmentRequest
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

	isAdmin := containsRole(identity.Roles(), "admin")
	result, err := h.svc.CreateAttachment(c.Request.Context(), id, identity.UserID(), isAdmin, tenantID, req)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.JSON(c, http.StatusCreated, result)
}

// ListAttachments handles GET /api/appointments/:id/attachments
func (h *Handler) ListAttachments(c *gin.Context) {
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

	isAdmin := containsRole(identity.Roles(), "admin")
	result, err := h.svc.ListAttachments(c.Request.Context(), id, identity.UserID(), isAdmin, tenantID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

// CreateAvailabilityRule handles POST /api/appointments/availability/rules
func (h *Handler) CreateAvailabilityRule(c *gin.Context) {
	var req transport.CreateAvailabilityRuleRequest
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

	isAdmin := containsRole(identity.Roles(), "admin")
	result, err := h.svc.CreateAvailabilityRule(c.Request.Context(), identity.UserID(), isAdmin, tenantID, req)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.JSON(c, http.StatusCreated, result)
}

// ListAvailabilityRules handles GET /api/appointments/availability/rules
func (h *Handler) ListAvailabilityRules(c *gin.Context) {
	var userID *uuid.UUID
	if raw := c.Query("userId"); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
			return
		}
		userID = &parsed
	}

	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	isAdmin := containsRole(identity.Roles(), "admin")
	result, err := h.svc.ListAvailabilityRules(c.Request.Context(), identity.UserID(), isAdmin, tenantID, userID)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

// DeleteAvailabilityRule handles DELETE /api/appointments/availability/rules/:id
func (h *Handler) DeleteAvailabilityRule(c *gin.Context) {
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

	isAdmin := containsRole(identity.Roles(), "admin")
	if err := h.svc.DeleteAvailabilityRule(c.Request.Context(), identity.UserID(), isAdmin, tenantID, id); httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, gin.H{"message": "availability rule deleted"})
}

// CreateAvailabilityOverride handles POST /api/appointments/availability/overrides
func (h *Handler) CreateAvailabilityOverride(c *gin.Context) {
	var req transport.CreateAvailabilityOverrideRequest
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

	isAdmin := containsRole(identity.Roles(), "admin")
	result, err := h.svc.CreateAvailabilityOverride(c.Request.Context(), identity.UserID(), isAdmin, tenantID, req)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.JSON(c, http.StatusCreated, result)
}

// ListAvailabilityOverrides handles GET /api/appointments/availability/overrides
func (h *Handler) ListAvailabilityOverrides(c *gin.Context) {
	var userID *uuid.UUID
	if raw := c.Query("userId"); raw != "" {
		parsed, err := uuid.Parse(raw)
		if err != nil {
			httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
			return
		}
		userID = &parsed
	}

	startDate := c.Query("startDate")
	endDate := c.Query("endDate")
	var startPtr *string
	var endPtr *string
	if startDate != "" {
		startPtr = &startDate
	}
	if endDate != "" {
		endPtr = &endDate
	}

	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	tenantID, ok := mustGetTenantID(c, identity)
	if !ok {
		return
	}

	isAdmin := containsRole(identity.Roles(), "admin")
	result, err := h.svc.ListAvailabilityOverrides(c.Request.Context(), identity.UserID(), isAdmin, tenantID, userID, startPtr, endPtr)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

// DeleteAvailabilityOverride handles DELETE /api/appointments/availability/overrides/:id
func (h *Handler) DeleteAvailabilityOverride(c *gin.Context) {
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

	isAdmin := containsRole(identity.Roles(), "admin")
	if err := h.svc.DeleteAvailabilityOverride(c.Request.Context(), identity.UserID(), isAdmin, tenantID, id); httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, gin.H{"message": "availability override deleted"})
}

// GetAvailableSlots handles GET /api/appointments/availability/slots
func (h *Handler) GetAvailableSlots(c *gin.Context) {
	var req transport.GetAvailableSlotsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, err.Error())
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

	isAdmin := containsRole(identity.Roles(), "admin")
	result, err := h.svc.GetAvailableSlots(c.Request.Context(), identity.UserID(), isAdmin, tenantID, req)
	if httpkit.HandleError(c, err) {
		return
	}

	httpkit.OK(c, result)
}

func containsRole(roles []string, role string) bool {
	for _, r := range roles {
		if r == role {
			return true
		}
	}
	return false
}
