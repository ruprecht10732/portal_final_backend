package tasks

import (
	"net/http"

	"portal_final_backend/platform/httpkit"
	"portal_final_backend/platform/validator"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	errOrganizationRequired = "organization required"
	errInvalidRequest       = "invalid request"
	errValidationFailed     = "validation failed"
	errInvalidTaskID        = "invalid task id"
)

type Handler struct {
	svc *Service
	val *validator.Validator
}

func NewHandler(svc *Service, val *validator.Validator) *Handler {
	return &Handler{svc: svc, val: val}
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("", h.List)
	rg.POST("", h.Create)
	rg.GET("/:id", h.Get)
	rg.PATCH("/:id", h.Update)
	rg.POST("/:id/assign", h.Assign)
	rg.POST("/:id/complete", h.Complete)
	rg.POST("/:id/cancel", h.Cancel)
}

func (h *Handler) List(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	if identity.TenantID() == nil {
		httpkit.Error(c, http.StatusForbidden, errOrganizationRequired, nil)
		return
	}
	var req ListTasksRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, errInvalidRequest, err.Error())
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, errValidationFailed, err.Error())
		return
	}
	items, err := h.svc.List(c.Request.Context(), *identity.TenantID(), req)
	if httpkit.HandleError(c, err) {
		return
	}
	httpkit.OK(c, gin.H{"items": items})
}

func (h *Handler) Create(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	if identity.TenantID() == nil {
		httpkit.Error(c, http.StatusForbidden, errOrganizationRequired, nil)
		return
	}
	var req CreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, errInvalidRequest, err.Error())
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, errValidationFailed, err.Error())
		return
	}
	item, err := h.svc.Create(c.Request.Context(), *identity.TenantID(), identity.UserID(), req)
	if httpkit.HandleError(c, err) {
		return
	}
	httpkit.JSON(c, http.StatusCreated, item)
}

func (h *Handler) Get(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	if identity.TenantID() == nil {
		httpkit.Error(c, http.StatusForbidden, errOrganizationRequired, nil)
		return
	}
	taskID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, errInvalidRequest, errInvalidTaskID)
		return
	}
	item, err := h.svc.Get(c.Request.Context(), *identity.TenantID(), taskID)
	if httpkit.HandleError(c, err) {
		return
	}
	httpkit.OK(c, item)
}

func (h *Handler) Update(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	if identity.TenantID() == nil {
		httpkit.Error(c, http.StatusForbidden, errOrganizationRequired, nil)
		return
	}
	taskID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, errInvalidRequest, errInvalidTaskID)
		return
	}
	var req UpdateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, errInvalidRequest, err.Error())
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, errValidationFailed, err.Error())
		return
	}
	item, err := h.svc.Update(c.Request.Context(), *identity.TenantID(), taskID, req)
	if httpkit.HandleError(c, err) {
		return
	}
	httpkit.OK(c, item)
}

func (h *Handler) Assign(c *gin.Context) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	if identity.TenantID() == nil {
		httpkit.Error(c, http.StatusForbidden, errOrganizationRequired, nil)
		return
	}
	taskID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, errInvalidRequest, errInvalidTaskID)
		return
	}
	var req AssignTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, errInvalidRequest, err.Error())
		return
	}
	if err := h.val.Struct(req); err != nil {
		httpkit.Error(c, http.StatusBadRequest, errValidationFailed, err.Error())
		return
	}
	assignedUserID, err := uuid.Parse(req.AssignedUserID)
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, errInvalidRequest, "invalid assigned user id")
		return
	}
	item, err := h.svc.Assign(c.Request.Context(), *identity.TenantID(), taskID, assignedUserID)
	if httpkit.HandleError(c, err) {
		return
	}
	httpkit.OK(c, item)
}

func (h *Handler) Complete(c *gin.Context) {
	h.handleStatusMutation(c, func(ctx *gin.Context, tenantID, taskID uuid.UUID) (TaskRecord, error) {
		return h.svc.Complete(ctx.Request.Context(), tenantID, taskID)
	})
}

func (h *Handler) Cancel(c *gin.Context) {
	h.handleStatusMutation(c, func(ctx *gin.Context, tenantID, taskID uuid.UUID) (TaskRecord, error) {
		return h.svc.Cancel(ctx.Request.Context(), tenantID, taskID)
	})
}

func (h *Handler) handleStatusMutation(c *gin.Context, action func(*gin.Context, uuid.UUID, uuid.UUID) (TaskRecord, error)) {
	identity := httpkit.MustGetIdentity(c)
	if identity == nil {
		return
	}
	if identity.TenantID() == nil {
		httpkit.Error(c, http.StatusForbidden, errOrganizationRequired, nil)
		return
	}
	taskID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpkit.Error(c, http.StatusBadRequest, errInvalidRequest, errInvalidTaskID)
		return
	}
	item, err := action(c, *identity.TenantID(), taskID)
	if httpkit.HandleError(c, err) {
		return
	}
	httpkit.OK(c, item)
}