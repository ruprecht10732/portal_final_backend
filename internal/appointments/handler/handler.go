package handler

import (
	"context" // Added missing import
	"net/http"

	"portal_final_backend/internal/appointments/service"
	"portal_final_backend/internal/appointments/transport"
	"portal_final_backend/platform/apperr"
	"portal_final_backend/platform/httpkit"
	"portal_final_backend/platform/validator"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	msgInvalidRequest   = "invalid request"
	msgValidationFailed = "validation failed"
)

type Handler struct {
	svc *service.Service
	val *validator.Validator
}

func New(svc *service.Service, val *validator.Validator) *Handler {
	return &Handler{svc: svc, val: val}
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("", h.List)
	rg.POST("", h.Create)
	rg.GET("/:id", h.GetByID)
	rg.PUT("/:id", h.Update)
	rg.DELETE("/:id", h.Delete)
	rg.PATCH("/:id/status", h.UpdateStatus)
	rg.GET("/:id/visit-report", h.GetVisitReport)
	rg.PUT("/:id/visit-report", h.UpsertVisitReport)
	rg.POST("/:id/attachments/presign", h.PresignAttachmentUpload)
	rg.GET("/:id/attachments", h.ListAttachments)
	rg.POST("/:id/attachments", h.CreateAttachment)
	rg.GET("/:id/attachments/:attachmentId/download", h.GetAttachmentDownloadURL)

	avail := rg.Group("/availability")
	{
		avail.GET("/rules", h.ListAvailabilityRules)
		avail.POST("/rules", h.CreateAvailabilityRule)
		avail.PUT("/rules/:id", h.UpdateAvailabilityRule)
		avail.DELETE("/rules/:id", h.DeleteAvailabilityRule)

		avail.GET("/overrides", h.ListAvailabilityOverrides)
		avail.POST("/overrides", h.CreateAvailabilityOverride)
		avail.PUT("/overrides/:id", h.UpdateAvailabilityOverride)
		avail.DELETE("/overrides/:id", h.DeleteAvailabilityOverride)

		avail.GET("/slots", h.GetAvailableSlots)
	}
}

// --- Appointments ---

func (h *Handler) List(c *gin.Context) {
	var req transport.ListAppointmentsRequest
	if !h.bind(c, &req, true) {
		return
	}

	ctx, auth, ok := h.reqCtx(c)
	if !ok {
		return
	}

	result, err := h.svc.List(ctx, auth.UserID, auth.IsAdmin, auth.TenantID, req)
	h.respond(c, result, err, http.StatusOK)
}

func (h *Handler) Create(c *gin.Context) {
	var req transport.CreateAppointmentRequest
	if !h.bind(c, &req, false) {
		return
	}

	ctx, auth, ok := h.reqCtx(c)
	if !ok {
		return
	}

	result, err := h.svc.Create(ctx, auth.UserID, auth.IsAdmin, auth.TenantID, req)
	h.respond(c, result, err, http.StatusCreated)
}

func (h *Handler) GetByID(c *gin.Context) {
	id, ok := h.pathID(c, "id")
	if !ok {
		return
	}

	ctx, auth, ok := h.reqCtx(c)
	if !ok {
		return
	}

	result, err := h.svc.GetByID(ctx, id, auth.UserID, auth.IsAdmin, auth.TenantID)
	h.respond(c, result, err, http.StatusOK)
}

func (h *Handler) Update(c *gin.Context) {
	id, ok := h.pathID(c, "id")
	if !ok {
		return
	}

	var req transport.UpdateAppointmentRequest
	if !h.bind(c, &req, false) {
		return
	}

	ctx, auth, ok := h.reqCtx(c)
	if !ok {
		return
	}

	result, err := h.svc.Update(ctx, id, auth.UserID, auth.IsAdmin, auth.TenantID, req)
	h.respond(c, result, err, http.StatusOK)
}

func (h *Handler) Delete(c *gin.Context) {
	id, ok := h.pathID(c, "id")
	if !ok {
		return
	}

	ctx, auth, ok := h.reqCtx(c)
	if !ok {
		return
	}

	err := h.svc.Delete(ctx, id, auth.UserID, auth.IsAdmin, auth.TenantID)
	h.respond(c, gin.H{"message": "appointment deleted"}, err, http.StatusOK)
}

func (h *Handler) UpdateStatus(c *gin.Context) {
	id, ok := h.pathID(c, "id")
	if !ok {
		return
	}

	var req transport.UpdateAppointmentStatusRequest
	if !h.bind(c, &req, false) {
		return
	}

	ctx, auth, ok := h.reqCtx(c)
	if !ok {
		return
	}

	result, err := h.svc.UpdateStatus(ctx, id, auth.UserID, auth.IsAdmin, auth.TenantID, req)
	h.respond(c, result, err, http.StatusOK)
}

// --- Visit Reports ---

func (h *Handler) GetVisitReport(c *gin.Context) {
	id, ok := h.pathID(c, "id")
	if !ok {
		return
	}

	ctx, auth, ok := h.reqCtx(c)
	if !ok {
		return
	}

	result, err := h.svc.GetVisitReport(ctx, id, auth.UserID, auth.IsAdmin, auth.TenantID)
	if err != nil {
		if domainErr, ok := err.(*apperr.Error); ok && domainErr.Kind == apperr.KindNotFound {
			httpkit.OK(c, nil)
			return
		}
		httpkit.HandleError(c, err)
		return
	}
	httpkit.OK(c, result)
}

func (h *Handler) UpsertVisitReport(c *gin.Context) {
	id, ok := h.pathID(c, "id")
	if !ok {
		return
	}

	var req transport.UpsertVisitReportRequest
	if !h.bind(c, &req, false) {
		return
	}

	ctx, auth, ok := h.reqCtx(c)
	if !ok {
		return
	}

	result, err := h.svc.UpsertVisitReport(ctx, id, auth.UserID, auth.IsAdmin, auth.TenantID, req)
	h.respond(c, result, err, http.StatusOK)
}

// --- Attachments ---

func (h *Handler) PresignAttachmentUpload(c *gin.Context) {
	id, ok := h.pathID(c, "id")
	if !ok {
		return
	}

	var req transport.PresignedUploadRequest
	if !h.bind(c, &req, false) {
		return
	}

	ctx, auth, ok := h.reqCtx(c)
	if !ok {
		return
	}

	result, err := h.svc.PresignAttachmentUpload(ctx, id, auth.UserID, auth.IsAdmin, auth.TenantID, req)
	h.respond(c, result, err, http.StatusOK)
}

func (h *Handler) CreateAttachment(c *gin.Context) {
	id, ok := h.pathID(c, "id")
	if !ok {
		return
	}

	var req transport.CreateAppointmentAttachmentRequest
	if !h.bind(c, &req, false) {
		return
	}

	ctx, auth, ok := h.reqCtx(c)
	if !ok {
		return
	}

	result, err := h.svc.CreateAttachment(ctx, id, auth.UserID, auth.IsAdmin, auth.TenantID, req)
	h.respond(c, result, err, http.StatusCreated)
}

func (h *Handler) ListAttachments(c *gin.Context) {
	id, ok := h.pathID(c, "id")
	if !ok {
		return
	}

	ctx, auth, ok := h.reqCtx(c)
	if !ok {
		return
	}

	result, err := h.svc.ListAttachments(ctx, id, auth.UserID, auth.IsAdmin, auth.TenantID)
	h.respond(c, result, err, http.StatusOK)
}

func (h *Handler) GetAttachmentDownloadURL(c *gin.Context) {
	id, ok := h.pathID(c, "id")
	if !ok {
		return
	}
	attID, ok := h.pathID(c, "attachmentId")
	if !ok {
		return
	}

	ctx, auth, ok := h.reqCtx(c)
	if !ok {
		return
	}

	result, err := h.svc.GetAttachmentDownloadURL(ctx, id, attID, auth.UserID, auth.IsAdmin, auth.TenantID)
	h.respond(c, result, err, http.StatusOK)
}

// --- Availability ---

func (h *Handler) CreateAvailabilityRule(c *gin.Context) {
	var req transport.CreateAvailabilityRuleRequest
	if !h.bind(c, &req, false) {
		return
	}

	ctx, auth, ok := h.reqCtx(c)
	if !ok {
		return
	}

	result, err := h.svc.CreateAvailabilityRule(ctx, auth.UserID, auth.IsAdmin, auth.TenantID, req)
	h.respond(c, result, err, http.StatusCreated)
}

func (h *Handler) ListAvailabilityRules(c *gin.Context) {
	var userID *uuid.UUID
	if raw := c.Query("userId"); raw != "" {
		if id, err := uuid.Parse(raw); err == nil {
			userID = &id
		}
	}

	ctx, auth, ok := h.reqCtx(c)
	if !ok {
		return
	}

	result, err := h.svc.ListAvailabilityRules(ctx, auth.UserID, auth.IsAdmin, auth.TenantID, userID)
	h.respond(c, result, err, http.StatusOK)
}

func (h *Handler) UpdateAvailabilityRule(c *gin.Context) {
	id, ok := h.pathID(c, "id")
	if !ok {
		return
	}

	var req transport.UpdateAvailabilityRuleRequest
	if !h.bind(c, &req, false) {
		return
	}

	ctx, auth, ok := h.reqCtx(c)
	if !ok {
		return
	}

	result, err := h.svc.UpdateAvailabilityRule(ctx, auth.UserID, auth.IsAdmin, auth.TenantID, id, req)
	h.respond(c, result, err, http.StatusOK)
}

func (h *Handler) DeleteAvailabilityRule(c *gin.Context) {
	id, ok := h.pathID(c, "id")
	if !ok {
		return
	}

	ctx, auth, ok := h.reqCtx(c)
	if !ok {
		return
	}

	err := h.svc.DeleteAvailabilityRule(ctx, auth.UserID, auth.IsAdmin, auth.TenantID, id)
	h.respond(c, gin.H{"message": "availability rule deleted"}, err, http.StatusOK)
}

func (h *Handler) ListAvailabilityOverrides(c *gin.Context) {
	ctx, auth, ok := h.reqCtx(c)
	if !ok {
		return
	}

	var userID *uuid.UUID
	if raw := c.Query("userId"); raw != "" {
		if id, err := uuid.Parse(raw); err == nil {
			userID = &id
		}
	}

	start, end := c.Query("startDate"), c.Query("endDate")
	var startPtr, endPtr *string
	if start != "" {
		startPtr = &start
	}
	if end != "" {
		endPtr = &end
	}

	result, err := h.svc.ListAvailabilityOverrides(ctx, auth.UserID, auth.IsAdmin, auth.TenantID, userID, startPtr, endPtr)
	h.respond(c, result, err, http.StatusOK)
}

func (h *Handler) CreateAvailabilityOverride(c *gin.Context) {
	var req transport.CreateAvailabilityOverrideRequest
	if !h.bind(c, &req, false) {
		return
	}

	ctx, auth, ok := h.reqCtx(c)
	if !ok {
		return
	}

	result, err := h.svc.CreateAvailabilityOverride(ctx, auth.UserID, auth.IsAdmin, auth.TenantID, req)
	h.respond(c, result, err, http.StatusCreated)
}

func (h *Handler) UpdateAvailabilityOverride(c *gin.Context) {
	id, ok := h.pathID(c, "id")
	if !ok {
		return
	}

	var req transport.UpdateAvailabilityOverrideRequest
	if !h.bind(c, &req, false) {
		return
	}

	ctx, auth, ok := h.reqCtx(c)
	if !ok {
		return
	}

	result, err := h.svc.UpdateAvailabilityOverride(ctx, auth.UserID, auth.IsAdmin, auth.TenantID, id, req)
	h.respond(c, result, err, http.StatusOK)
}

func (h *Handler) DeleteAvailabilityOverride(c *gin.Context) {
	id, ok := h.pathID(c, "id")
	if !ok {
		return
	}

	ctx, auth, ok := h.reqCtx(c)
	if !ok {
		return
	}

	err := h.svc.DeleteAvailabilityOverride(ctx, auth.UserID, auth.IsAdmin, auth.TenantID, id)
	h.respond(c, gin.H{"message": "availability override deleted"}, err, http.StatusOK)
}

func (h *Handler) GetAvailableSlots(c *gin.Context) {
	var req transport.GetAvailableSlotsRequest
	if !h.bind(c, &req, true) {
		return
	}

	ctx, auth, ok := h.reqCtx(c)
	if !ok {
		return
	}

	result, err := h.svc.GetAvailableSlots(ctx, auth.UserID, auth.IsAdmin, auth.TenantID, req)
	h.respond(c, result, err, http.StatusOK)
}

// --- Helpers ---

type authParams struct {
	UserID   uuid.UUID
	TenantID uuid.UUID
	IsAdmin  bool
}

func (h *Handler) reqCtx(c *gin.Context) (context.Context, authParams, bool) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return nil, authParams{}, false
	}

	tenantID := identity.TenantID()
	if tenantID == nil {
		httpkit.Error(c, http.StatusBadRequest, "tenant ID is required", nil)
		return nil, authParams{}, false
	}

	isAdmin := false
	for _, r := range identity.Roles() {
		if r == "admin" {
			isAdmin = true
			break
		}
	}

	return c.Request.Context(), authParams{
		UserID:   identity.UserID(),
		TenantID: *tenantID,
		IsAdmin:  isAdmin,
	}, true
}

func (h *Handler) pathID(c *gin.Context, key string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(key))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, nil)
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) bind(c *gin.Context, req interface{}, isQuery bool) bool {
	var err error
	if isQuery {
		err = c.ShouldBindQuery(req)
	} else {
		err = c.ShouldBindJSON(req)
	}

	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgInvalidRequest, err.Error())
		return false
	}

	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, msgValidationFailed, err.Error())
		return false
	}
	return true
}

func (h *Handler) respond(c *gin.Context, data interface{}, err error, successStatus int) {
	if httpkit.HandleError(c, err) {
		return
	}
	if successStatus == http.StatusCreated {
		httpkit.JSON(c, http.StatusCreated, data)
	} else {
		httpkit.OK(c, data)
	}
}
